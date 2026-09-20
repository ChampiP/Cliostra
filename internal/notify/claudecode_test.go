package notify

import (
	"bufio"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"
)

// listenTestSocket levanta un listener Unix en un directorio temporal y
// devuelve su ruta junto con el primer *bufio.Reader conectado.
func listenTestSocket(t *testing.T) (string, func() *bufio.Reader) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cc.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	accept := func() *bufio.Reader {
		conn, err := ln.Accept()
		if err != nil {
			t.Fatalf("Accept: %v", err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		return bufio.NewReader(conn)
	}
	return path, accept
}

func readLine(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("ReadString: %v", err)
	}
	return line
}

func TestSendWithTokenWritesAuthThenMessage(t *testing.T) {
	path, accept := listenTestSocket(t)

	done := make(chan error, 1)
	go func() { done <- Send(Config{SocketPath: path, Token: "tok-123"}, "hola mundo") }()

	r := accept()

	authLine := readLine(t, r)
	if authLine[len(authLine)-1] != '\n' {
		t.Fatalf("la línea de auth debe terminar en \\n: %q", authLine)
	}
	var auth authMessage
	if err := json.Unmarshal([]byte(authLine[:len(authLine)-1]), &auth); err != nil {
		t.Fatalf("json auth inválido: %v", err)
	}
	if auth.Type != "auth" || auth.Token != "tok-123" {
		t.Fatalf("auth inesperado: %+v", auth)
	}

	msgLine := readLine(t, r)
	if msgLine[len(msgLine)-1] != '\n' {
		t.Fatalf("la línea de mensaje debe terminar en \\n: %q", msgLine)
	}
	var msg userMessage
	if err := json.Unmarshal([]byte(msgLine[:len(msgLine)-1]), &msg); err != nil {
		t.Fatalf("json mensaje inválido: %v", err)
	}
	if msg.Type != "user" || msg.Message.Role != "user" || msg.Message.Content != "hola mundo" {
		t.Fatalf("mensaje inesperado: %+v", msg)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Send devolvió error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Send no terminó a tiempo")
	}
}

func TestSendWithoutTokenSkipsAuthLine(t *testing.T) {
	path, accept := listenTestSocket(t)

	done := make(chan error, 1)
	go func() { done <- Send(Config{SocketPath: path}, "sin token") }()

	r := accept()

	// Sin token, la primera línea ya debe ser el mensaje de usuario, no auth.
	line := readLine(t, r)
	var msg userMessage
	if err := json.Unmarshal([]byte(line[:len(line)-1]), &msg); err != nil {
		t.Fatalf("json mensaje inválido: %v", err)
	}
	if msg.Type != "user" || msg.Message.Role != "user" || msg.Message.Content != "sin token" {
		t.Fatalf("mensaje inesperado (¿se mandó auth de más?): %+v", msg)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Send devolvió error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Send no terminó a tiempo")
	}
}

func TestSendFailsFastWhenSocketDoesNotExist(t *testing.T) {
	cfg := Config{SocketPath: filepath.Join(t.TempDir(), "no-existe.sock")}

	done := make(chan error, 1)
	go func() { done <- Send(cfg, "nadie escucha") }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Send debía fallar contra un socket inexistente")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Send se colgó en vez de fallar rápido con socket inexistente")
	}
}

func TestConfigAvailable(t *testing.T) {
	if (Config{}).Available() {
		t.Fatal("Config vacía no debería estar disponible")
	}
	if !(Config{SocketPath: "/tmp/x.sock"}).Available() {
		t.Fatal("Config con SocketPath debería estar disponible")
	}
}
