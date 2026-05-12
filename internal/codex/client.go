package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
)

type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	nextID         atomic.Int64
	pending        map[int64]chan rpcMessage
	events         chan rpcMessage
	errs           chan error
	requestHandler ServerRequestHandler
	mu             sync.Mutex
	closed         chan struct{}
}

type rpcMessage struct {
	ID     *int64          `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type Event struct {
	Method string
	Params json.RawMessage
}

type ServerRequest struct {
	ID     int64
	Method string
	Params json.RawMessage
}

type ServerRequestHandler func(context.Context, ServerRequest) (any, error)

type StartOptions struct {
	CLIPath    string
	WorkingDir string
}

func StartStdio(ctx context.Context) (*Client, error) {
	return StartStdioWithPath(ctx, "")
}

func StartStdioWithPath(ctx context.Context, cliPath string) (*Client, error) {
	return StartStdioWithOptions(ctx, StartOptions{CLIPath: cliPath})
}

func StartStdioWithOptions(ctx context.Context, opts StartOptions) (*Client, error) {
	codexPath, err := resolveCodexPath(opts.CLIPath)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, codexPath, "app-server", "--listen", "stdio://")
	if opts.WorkingDir != "" {
		cmd.Dir = opts.WorkingDir
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	c := &Client{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		pending: make(map[int64]chan rpcMessage),
		events:  make(chan rpcMessage, 256),
		errs:    make(chan error, 8),
		closed:  make(chan struct{}),
	}
	go c.readStdout()
	go c.readStderr()
	go c.wait()

	if err := c.Initialize(ctx); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

func resolveCodexPath(cliPath string) (string, error) {
	if cliPath != "" {
		return cliPath, nil
	}
	if p, err := exec.LookPath("codex"); err == nil {
		return p, nil
	}
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		candidate := filepath.Join(localAppData, "OpenAI", "Codex", "bin", "codex.exe")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "codex", nil
}

func (c *Client) Initialize(ctx context.Context) error {
	params := map[string]any{
		"clientInfo": map[string]any{
			"name":    "dexgram",
			"title":   nil,
			"version": "0.0.1",
		},
		"capabilities": map[string]any{
			"experimentalApi": true,
		},
	}
	var result map[string]any
	if err := c.Call(ctx, "initialize", params, &result); err != nil {
		return err
	}
	return c.Notify("initialized", nil)
}

func (c *Client) Events() <-chan Event {
	out := make(chan Event)
	go func() {
		defer close(out)
		for msg := range c.events {
			out <- Event{Method: msg.Method, Params: msg.Params}
		}
	}()
	return out
}

func (c *Client) Errors() <-chan error {
	return c.errs
}

func (c *Client) SetServerRequestHandler(handler ServerRequestHandler) {
	c.mu.Lock()
	c.requestHandler = handler
	c.mu.Unlock()
}

func (c *Client) Call(ctx context.Context, method string, params any, out any) error {
	id := c.nextID.Add(1)
	ch := make(chan rpcMessage, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	req := map[string]any{
		"id":     id,
		"method": method,
		"params": params,
		"trace":  nil,
	}
	if err := c.writeJSON(req); err != nil {
		c.forget(id)
		return err
	}

	select {
	case msg := <-ch:
		if msg.Error != nil {
			return fmt.Errorf("%s: %s", method, msg.Error.Message)
		}
		if out == nil {
			return nil
		}
		if len(msg.Result) == 0 || string(msg.Result) == "null" {
			return nil
		}
		return json.Unmarshal(msg.Result, out)
	case <-ctx.Done():
		c.forget(id)
		return ctx.Err()
	case <-c.closed:
		return errors.New("codex app-server closed")
	}
}

func (c *Client) Notify(method string, params any) error {
	req := map[string]any{"method": method}
	if params != nil {
		req["params"] = params
	}
	return c.writeJSON(req)
}

func (c *Client) Close() error {
	select {
	case <-c.closed:
		return nil
	default:
	}
	_ = c.stdin.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	<-c.closed
	return nil
}

func (c *Client) forget(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *Client) writeJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = c.stdin.Write(b)
	return err
}

func (c *Client) readStdout() {
	scanner := bufio.NewScanner(c.stdout)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var msg rpcMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			c.errs <- fmt.Errorf("decode app-server line: %w", err)
			continue
		}
		if msg.ID != nil {
			if msg.Method != "" {
				c.handleServerRequest(*msg.ID, msg.Method, msg.Params)
				continue
			}
			c.mu.Lock()
			ch := c.pending[*msg.ID]
			delete(c.pending, *msg.ID)
			c.mu.Unlock()
			if ch != nil {
				ch <- msg
			}
			continue
		}
		if msg.Method != "" {
			c.events <- msg
		}
	}
	if err := scanner.Err(); err != nil {
		select {
		case <-c.closed:
		default:
			c.errs <- fmt.Errorf("read app-server stdout: %w", err)
		}
	}
	close(c.events)
}

func (c *Client) handleServerRequest(id int64, method string, params json.RawMessage) {
	c.mu.Lock()
	handler := c.requestHandler
	c.mu.Unlock()
	if handler == nil {
		c.replyUnsupported(id, method)
		return
	}
	go func() {
		result, err := handler(context.Background(), ServerRequest{
			ID:     id,
			Method: method,
			Params: params,
		})
		if err != nil {
			c.replyError(id, -32000, err.Error())
			return
		}
		c.replyResult(id, result)
	}()
}

func (c *Client) replyUnsupported(id int64, method string) {
	c.replyError(id, -32601, "dexgram does not handle server request: "+method)
}

func (c *Client) replyError(id int64, code int, message string) {
	_ = c.writeJSON(map[string]any{
		"id": id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

func (c *Client) replyResult(id int64, result any) {
	_ = c.writeJSON(map[string]any{
		"id":     id,
		"result": result,
	})
}

func (c *Client) readStderr() {
	scanner := bufio.NewScanner(c.stderr)
	for scanner.Scan() {
		c.errs <- fmt.Errorf("app-server stderr: %s", scanner.Text())
	}
}

func (c *Client) wait() {
	_ = c.cmd.Wait()
	close(c.closed)
}
