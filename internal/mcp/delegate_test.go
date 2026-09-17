package mcp

import (
	"bufio"
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ChampiP/Cliostra/internal/api"
	"github.com/ChampiP/Cliostra/internal/notify"
)

func TestMCPDelegateFailsWithoutClaudeCodeSession(t *testing.T) {
	repo := initTestRepo(t)
	rt := newTestRuntime(t)
	dial := rpcDialer(t, rt)
	// notify.Config{} vacía simula correr fuera de una sesión de Claude Code.
	cs := connectedClient(t, dial)

	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "delegate", Arguments: map[string]any{
		"adapter": "fake", "repo": repo, "prompt": "hola", "read_only": true,
	}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("delegate sin sesión de Claude Code debería fallar de entrada")
	}
	if len(res.Content) == 0 {
		t.Fatal("el error debería explicar por qué falló")
	}
}

func TestMCPDelegateReturnsImmediatelyAndNotifiesOnCompletion(t *testing.T) {
	repo := initTestRepo(t)
	rt := newTestRuntime(t)
	dial := rpcDialer(t, rt)

	sockPath := filepath.Join(t.TempDir(), "cc.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	cs := connectedClientWithNotify(t, dial, notify.Config{SocketPath: sockPath, Token: "tok"})

	begin := time.Now()
	start := callTool[api.StartResponse](t, cs, "delegate", map[string]any{
		"adapter": "fake", "repo": repo, "prompt": "hola", "read_only": true,
	})
	if time.Since(begin) > 2*time.Second {
		t.Fatal("delegate debe devolver el control de inmediato, sin esperar al trabajo")
	}
	if start.ID == "" {
		t.Fatal("delegate debe devolver un id")
	}

	conn, err := ln.Accept()
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	r := bufio.NewReader(conn)

	// Primera línea: auth (se mandó token).
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("no llegó la línea de auth: %v", err)
	}
	// Segunda línea: el aviso de finalización.
	msgLine, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("no llegó el aviso de finalización: %v", err)
	}
	if !strings.Contains(msgLine, start.ID) {
		t.Fatalf("el aviso debería mencionar el id %q: %s", start.ID, msgLine)
	}
	if !strings.Contains(msgLine, "fake") {
		t.Fatalf("el aviso debería mencionar el adaptador: %s", msgLine)
	}
	if !strings.Contains(msgLine, "succeeded") {
		t.Fatalf("el aviso debería mencionar el estado final: %s", msgLine)
	}
}
