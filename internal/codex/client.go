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
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
)

const maxAppServerStderrLineLen = 2000
const maxAppServerParseBufferLen = 1024 * 1024
const maxAppServerParseBufferLines = 1000

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

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
	readers        sync.WaitGroup
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

type pendingParse struct {
	text      string
	lineCount int
	firstErr  error
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
	c.readers.Add(2)
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
			return fmt.Errorf("app-server returned error for %s (code %d): %s", method, msg.Error.Code, msg.Error.Message)
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
	defer c.readers.Done()
	scanner := bufio.NewScanner(c.stdout)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 16*1024*1024)
	var pending *pendingParse
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if pending != nil {
			msg, nextPending, err, ok := parsePendingAppServerLine(pending, line)
			pending = nextPending
			if err != nil {
				c.reportError(err)
			}
			if !ok {
				continue
			}
			c.handleStdoutMessage(msg)
			continue
		}
		msg, nextPending, err, ok := parseAppServerLine(line)
		pending = nextPending
		if err != nil {
			c.reportError(err)
		}
		if !ok {
			continue
		}
		c.handleStdoutMessage(msg)
	}
	if err := scanner.Err(); err != nil {
		if !isExpectedAppServerPipeClose(err) {
			select {
			case <-c.closed:
			default:
				c.reportError(fmt.Errorf("read app-server stdout: %w", err))
			}
		}
	}
	close(c.events)
}

func parseAppServerLine(line string) (rpcMessage, *pendingParse, error, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return rpcMessage{}, nil, nil, false
	}
	var msg rpcMessage
	if err := json.Unmarshal([]byte(trimmed), &msg); err != nil {
		if shouldBufferAppServerParseFailure(trimmed, err) {
			return rpcMessage{}, &pendingParse{text: trimmed, lineCount: 1, firstErr: err}, nil, false
		}
		return rpcMessage{}, nil, fmt.Errorf("decode app-server line: %w", err), false
	}
	return msg, nil, nil, true
}

func parsePendingAppServerLine(pending *pendingParse, line string) (rpcMessage, *pendingParse, error, bool) {
	candidate := pending.text + `\n` + line
	var msg rpcMessage
	if err := json.Unmarshal([]byte(candidate), &msg); err != nil {
		lineCount := pending.lineCount + 1
		if len(candidate) <= maxAppServerParseBufferLen && lineCount <= maxAppServerParseBufferLines {
			return rpcMessage{}, &pendingParse{text: candidate, lineCount: lineCount, firstErr: pending.firstErr}, nil, false
		}
		return rpcMessage{}, nil, fmt.Errorf("decode buffered app-server line after %d fragments: %w", lineCount, err), false
	}
	return msg, nil, nil, true
}

func shouldBufferAppServerParseFailure(line string, err error) bool {
	if !strings.HasPrefix(line, "{") && !strings.HasPrefix(line, "[") {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "unexpected end of JSON input") ||
		strings.Contains(msg, "invalid character") && strings.Contains(msg, "in string literal")
}

func (c *Client) handleStdoutMessage(msg rpcMessage) {
	if msg.ID != nil {
		if msg.Method != "" {
			c.handleServerRequest(*msg.ID, msg.Method, msg.Params)
			return
		}
		c.mu.Lock()
		ch := c.pending[*msg.ID]
		delete(c.pending, *msg.ID)
		c.mu.Unlock()
		if ch != nil {
			ch <- msg
		}
		return
	}
	if msg.Method != "" {
		c.events <- msg
	}
}

func isExpectedAppServerPipeClose(err error) bool {
	return errors.Is(err, os.ErrClosed) ||
		errors.Is(err, io.ErrClosedPipe) ||
		strings.Contains(strings.ToLower(err.Error()), "file already closed")
}

func normalizeAppServerStderr(line string) string {
	line = strings.TrimSpace(ansiEscapePattern.ReplaceAllString(line, ""))
	if line == "" {
		return "app-server stderr: <blank>"
	}

	var entry struct {
		Level  string `json:"level"`
		Target string `json:"target"`
		Fields struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		} `json:"fields"`
	}
	if json.Unmarshal([]byte(line), &entry) == nil && (entry.Level != "" || entry.Target != "") {
		msg := entry.Fields.Message
		if msg == "" {
			msg = entry.Fields.Error
		}
		if msg == "" {
			msg = line
		}
		if len(msg) > maxAppServerStderrLineLen {
			msg = msg[:maxAppServerStderrLineLen] + fmt.Sprintf("... [truncated %d bytes]", len(msg)-maxAppServerStderrLineLen)
		}
		level := strings.ToLower(entry.Level)
		if level == "" {
			level = "stderr"
		}
		if entry.Target == "" {
			return fmt.Sprintf("app-server %s: %s", level, msg)
		}
		return fmt.Sprintf("app-server %s %s: %s", level, entry.Target, msg)
	}

	if len(line) > maxAppServerStderrLineLen {
		line = line[:maxAppServerStderrLineLen] + fmt.Sprintf("... [truncated %d bytes]", len(line)-maxAppServerStderrLineLen)
	}
	if strings.Contains(line, "codex_core::tools::router") && strings.Contains(line, "ERROR") {
		if _, after, ok := strings.Cut(line, "error="); ok {
			return "app-server tool error: " + strings.TrimSpace(after)
		}
		return "app-server tool error: " + line
	}
	return "app-server stderr: " + line
}

func (c *Client) reportError(err error) {
	select {
	case <-c.closed:
		select {
		case c.errs <- err:
		default:
		}
	default:
		select {
		case c.errs <- err:
		default:
		}
	}
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
	defer c.readers.Done()
	scanner := bufio.NewScanner(c.stderr)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 16*1024*1024)
	for scanner.Scan() {
		c.reportError(errors.New(normalizeAppServerStderr(scanner.Text())))
	}
	if err := scanner.Err(); err != nil && !isExpectedAppServerPipeClose(err) {
		select {
		case <-c.closed:
		default:
			c.reportError(fmt.Errorf("read app-server stderr: %w", err))
		}
	}
}

func (c *Client) wait() {
	_ = c.cmd.Wait()
	close(c.closed)
	c.readers.Wait()
	close(c.errs)
}
