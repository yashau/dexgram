package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestResolveCodexPathHonorsExplicitPath(t *testing.T) {
	got, err := resolveCodexPath(`C:\Codex\codex.exe`)
	if err != nil {
		t.Fatal(err)
	}
	if got != `C:\Codex\codex.exe` {
		t.Fatalf("resolveCodexPath returned %q", got)
	}
}

func TestResolveCodexPathFallsBackToLocalAppDataInstall(t *testing.T) {
	localAppData := t.TempDir()
	candidate := filepath.Join(localAppData, "OpenAI", "Codex", "bin", "codex.exe")
	if err := os.MkdirAll(filepath.Dir(candidate), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidate, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv("LOCALAPPDATA", localAppData)

	got, err := resolveCodexPath("")
	if err != nil {
		t.Fatal(err)
	}
	if got != candidate {
		t.Fatalf("resolveCodexPath returned %q, want %q", got, candidate)
	}
}

func TestClientCallWritesRequestAndReceivesResult(t *testing.T) {
	client, stdinReader, stdoutWriter := newPipeClient(t)
	defer func() {
		_ = stdoutWriter.Close()
	}()

	done := make(chan error, 1)
	go func() {
		req, err := readRPCLine(stdinReader)
		if err != nil {
			done <- err
			return
		}
		if req.Method != "thread/start" {
			done <- errors.New("unexpected method: " + req.Method)
			return
		}
		if !strings.Contains(string(req.Params), `"prompt":"hello"`) {
			done <- errors.New("request params missing prompt")
			return
		}
		if req.ID == nil {
			done <- errors.New("request missing id")
			return
		}
		_, err = io.WriteString(stdoutWriter, `{"id":`+itoa(*req.ID)+`,"result":{"ok":true}}`+"\n")
		done <- err
	}()

	var result struct {
		OK bool `json:"ok"`
	}
	if err := client.Call(context.Background(), "thread/start", map[string]any{"prompt": "hello"}, &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("result was not decoded: %#v", result)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestClientCallReturnsRPCErrorAndForgetsPending(t *testing.T) {
	client, stdinReader, stdoutWriter := newPipeClient(t)
	defer func() {
		_ = stdoutWriter.Close()
	}()

	go func() {
		req, err := readRPCLine(stdinReader)
		if err != nil {
			return
		}
		_, _ = io.WriteString(stdoutWriter, `{"id":`+itoa(*req.ID)+`,"error":{"code":-32000,"message":"boom"}}`+"\n")
	}()

	err := client.Call(context.Background(), "turn/start", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "app-server returned error for turn/start (code -32000): boom") {
		t.Fatalf("expected RPC error, got %v", err)
	}
	if len(client.pending) != 0 {
		t.Fatalf("expected pending request to be removed, got %#v", client.pending)
	}
}

func TestClientCallForgetsPendingOnContextCancel(t *testing.T) {
	client, stdinReader, stdoutWriter := newPipeClient(t)
	defer func() {
		_ = stdoutWriter.Close()
	}()
	defer func() {
		_ = stdinReader.Close()
	}()

	go func() {
		_, _ = readRPCLine(stdinReader)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := client.Call(ctx, "slow", nil, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if len(client.pending) != 0 {
		t.Fatalf("expected pending request to be removed, got %#v", client.pending)
	}
}

func TestClientRoutesEvents(t *testing.T) {
	client, _, stdoutWriter := newPipeClient(t)
	defer func() {
		_ = stdoutWriter.Close()
	}()

	_, err := io.WriteString(stdoutWriter, `{"method":"turn/completed","params":{"threadId":"thread-1"}}`+"\n")
	if err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-client.events:
		if msg.Method != "turn/completed" || !strings.Contains(string(msg.Params), "thread-1") {
			t.Fatalf("unexpected event: %#v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestClientRoutesEventWithRawNewlineInJSONString(t *testing.T) {
	client, _, stdoutWriter := newPipeClient(t)
	defer func() {
		_ = stdoutWriter.Close()
	}()

	_, err := io.WriteString(stdoutWriter, "{\"method\":\"item/commandExecution/outputDelta\",\"params\":{\"turnId\":\"turn-1\",\"delta\":\"first\nsecond\"}}\n")
	if err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-client.events:
		if msg.Method != "item/commandExecution/outputDelta" {
			t.Fatalf("unexpected method: %s", msg.Method)
		}
		var params struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			t.Fatal(err)
		}
		if params.Delta != "first\nsecond" {
			t.Fatalf("unexpected delta: %q", params.Delta)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestClientHandlesServerRequests(t *testing.T) {
	client, stdinReader, stdoutWriter := newPipeClient(t)
	defer func() {
		_ = stdoutWriter.Close()
	}()
	client.SetServerRequestHandler(func(_ context.Context, req ServerRequest) (any, error) {
		if req.Method != "approval/request" {
			t.Fatalf("unexpected method %q", req.Method)
		}
		return map[string]any{"approved": true}, nil
	})

	if _, err := io.WriteString(stdoutWriter, `{"id":42,"method":"approval/request","params":{"command":"go test"}}`+"\n"); err != nil {
		t.Fatal(err)
	}

	resp, err := readRPCLine(stdinReader)
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID == nil || *resp.ID != 42 {
		t.Fatalf("unexpected response id: %#v", resp.ID)
	}
	if !strings.Contains(string(resp.Result), `"approved":true`) {
		t.Fatalf("unexpected response result: %s", resp.Result)
	}
}

func TestExpectedAppServerPipeClose(t *testing.T) {
	for _, err := range []error{
		io.ErrClosedPipe,
		&os.PathError{Op: "read", Path: "|0", Err: os.ErrClosed},
		fmt.Errorf("read |0: file already closed"),
	} {
		if !isExpectedAppServerPipeClose(err) {
			t.Fatalf("expected %v to be treated as a normal pipe close", err)
		}
	}
	if isExpectedAppServerPipeClose(errors.New("connection reset by peer")) {
		t.Fatal("connection reset should still be reported")
	}
}

func TestNormalizeAppServerStderr(t *testing.T) {
	line := `{"timestamp":"2026-05-13T10:22:11Z","level":"WARN","fields":{"message":"startup remote plugin sync failed"},"target":"codex_core_plugins::manifest"}`
	got := normalizeAppServerStderr(line)
	want := "app-server warn codex_core_plugins::manifest: startup remote plugin sync failed"
	if got != want {
		t.Fatalf("normalizeAppServerStderr returned %q, want %q", got, want)
	}

	colored := "\x1b[31mERROR\x1b[0m codex_core::tools::router: error=Exit code: 1"
	got = normalizeAppServerStderr(colored)
	want = "app-server tool error: Exit code: 1"
	if got != want {
		t.Fatalf("normalizeAppServerStderr returned %q, want %q", got, want)
	}

	long := strings.Repeat("x", maxAppServerStderrLineLen+10)
	if got := normalizeAppServerStderr(long); !strings.Contains(got, "[truncated 10 bytes]") {
		t.Fatalf("expected long stderr to be truncated, got %q", got)
	}
}

func newPipeClient(t *testing.T) (*Client, *io.PipeReader, *io.PipeWriter) {
	t.Helper()
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	client := &Client{
		stdin:   stdinWriter,
		stdout:  stdoutReader,
		stderr:  io.NopCloser(strings.NewReader("")),
		pending: map[int64]chan rpcMessage{},
		events:  make(chan rpcMessage, 16),
		errs:    make(chan error, 16),
		closed:  make(chan struct{}),
	}
	client.readers.Add(1)
	go client.readStdout()
	return client, stdinReader, stdoutWriter
}

func readRPCLine(r io.Reader) (rpcMessage, error) {
	line, err := bufio.NewReader(r).ReadBytes('\n')
	if err != nil {
		return rpcMessage{}, err
	}
	var msg rpcMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		return rpcMessage{}, err
	}
	return msg, nil
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
