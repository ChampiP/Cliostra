package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"strings"
	"testing"
)

func TestWriteReadFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	req := StartRequest{Adapter: "claude-code", Repo: "/repo", Prompt: "hola", ReadOnly: true, Caller: "opencode", SessionID: "s-123", JobID: "j-456"}
	if err := WriteFrame(&buf, req); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	r := bufio.NewReader(&buf)
	var got StartRequest
	if err := ReadFrame(r, &got); err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if got != req {
		t.Fatalf("round trip mismatch: got %+v want %+v", got, req)
	}
}

func TestDetectHost(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "empty", env: map[string]string{}, want: ""},
		{name: "explicit host", env: map[string]string{"CLIOSTRA_HOST": "codex"}, want: "codex"},
		{name: "claude messaging socket", env: map[string]string{"CLAUDE_CODE_MESSAGING_SOCKET": "/tmp/cc.sock"}, want: "claude-code"},
		{name: "claude entrypoint", env: map[string]string{"CLAUDE_CODE_ENTRYPOINT": "cli"}, want: "claude-code"},
		{name: "claude project dir", env: map[string]string{"CLAUDE_PROJECT_DIR": "/home/user/proj"}, want: "claude-code"},
		{name: "opencode env", env: map[string]string{"OPENCODE": "1"}, want: "opencode"},
		{name: "opencode session", env: map[string]string{"OPENCODE_SESSION_ID": "ses_abc"}, want: "opencode"},
		{name: "agy agent", env: map[string]string{"ANTIGRAVITY_AGENT": "1"}, want: "agy"},
		{name: "gemini cli", env: map[string]string{"GEMINI_CLI": "1"}, want: "agy"},
		{name: "codex session", env: map[string]string{"CODEX_SESSION_ID": "codex_123"}, want: "codex"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectHost(func(k string) (string, bool) {
				v, ok := tc.env[k]
				return v, ok
			})
			if got != tc.want {
				t.Fatalf("DetectHost() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWriteFrameRejectsOversized(t *testing.T) {
	var buf bytes.Buffer
	big := StartRequest{Prompt: strings.Repeat("a", MaxFrameSize)}
	err := WriteFrame(&buf, big)
	if err == nil {
		t.Fatal("expected error for oversized frame")
	}
	apiErr, ok := err.(*Error)
	if !ok || apiErr.Code != ErrFrameTooLarge {
		t.Fatalf("expected ErrFrameTooLarge, got %v", err)
	}
}

func TestReadFrameRejectsOversizedLine(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(strings.Repeat("a", MaxFrameSize+10))
	buf.WriteByte('\n')
	r := bufio.NewReader(&buf)
	var v map[string]any
	err := ReadFrame(r, &v)
	if err == nil {
		t.Fatal("expected error for oversized incoming line")
	}
	apiErr, ok := err.(*Error)
	if !ok || apiErr.Code != ErrFrameTooLarge {
		t.Fatalf("expected ErrFrameTooLarge, got %v", err)
	}
}

func TestErrorShape(t *testing.T) {
	e := NewError(ErrNotFound, "no existe")
	if e.Code != ErrNotFound || e.Message != "no existe" {
		t.Fatalf("unexpected error shape: %+v", e)
	}
	if e.Error() == "" {
		t.Fatal("Error() should not be empty")
	}
}

func TestCallAndServeRoundTrip(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	handlers := map[string]Handler{
		"echo": func(payload json.RawMessage) (any, *Error) {
			var req StartRequest
			if err := json.Unmarshal(payload, &req); err != nil {
				return nil, NewError(ErrInvalidArgument, err.Error())
			}
			return StartResponse{ID: req.Adapter}, nil
		},
	}
	go func() {
		_ = Serve(server, handlers)
	}()

	var resp StartResponse
	if err := Call(client, "echo", StartRequest{Adapter: "claude-code"}, &resp); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.ID != "claude-code" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestCallUnknownMethodReturnsStructuredError(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		_ = Serve(server, map[string]Handler{})
	}()

	var resp StartResponse
	err := Call(client, "nope", StartRequest{}, &resp)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*Error)
	if !ok || apiErr.Code != ErrInvalidArgument {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}
