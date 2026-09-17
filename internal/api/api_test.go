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
	req := StartRequest{Adapter: "claude-code", Repo: "/repo", Prompt: "hola", ReadOnly: true}
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
