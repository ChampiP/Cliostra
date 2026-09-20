package mcp

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ChampiP/Cliostra/internal/adapters"
	"github.com/ChampiP/Cliostra/internal/api"
	"github.com/ChampiP/Cliostra/internal/notify"
	"github.com/ChampiP/Cliostra/internal/runtime"
)

// fakeAdapter simula un proveedor sin depender de Claude Code ni agy.
type fakeAdapter struct{}

func (fakeAdapter) Name() string { return "fake" }
func (fakeAdapter) Capabilities() adapters.Capabilities {
	return adapters.Capabilities{Cancel: false}
}
func (fakeAdapter) Build(req adapters.StartRequest, worktreeDir string) (adapters.ProcessSpec, error) {
	return adapters.ProcessSpec{Path: "/bin/sh", Args: []string{"-c", "cat >/dev/null; echo ok"}, Dir: worktreeDir, Stdin: []byte(req.Prompt)}, nil
}
func (fakeAdapter) Result(raw []byte) ([]byte, error) {
	return []byte(strings.TrimSpace(string(raw))), nil
}

type failingAdapter struct{}

func (failingAdapter) Name() string { return "failing" }
func (failingAdapter) Capabilities() adapters.Capabilities {
	return adapters.Capabilities{Cancel: false}
}
func (failingAdapter) Build(req adapters.StartRequest, worktreeDir string) (adapters.ProcessSpec, error) {
	return adapters.ProcessSpec{Path: "/bin/sh", Args: []string{"-c", "exit 1"}, Dir: worktreeDir}, nil
}
func (failingAdapter) Result(raw []byte) ([]byte, error) { return raw, nil }

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hola"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "f.txt")
	run("commit", "-m", "init")
	return dir
}

// rpcDialer arranca un servidor RPC en memoria (api.Serve) sobre un
// net.Pipe por conexión, delegando a un runtime.Runtime real pero sin
// proveedores reales (solo fakeAdapter).
func rpcDialer(t *testing.T, rt *runtime.Runtime) Dialer {
	t.Helper()
	handlers := runtime.Handlers(rt)
	return func() (net.Conn, error) {
		serverConn, clientConn := net.Pipe()
		go func() { _ = api.Serve(serverConn, handlers) }()
		return clientConn, nil
	}
}

func newTestRuntime(t *testing.T) *runtime.Runtime {
	t.Helper()
	rt, err := runtime.New(runtime.Config{
		StateDir:     t.TempDir(),
		WorktreeRoot: t.TempDir(),
		Workers:      2,
		Adapters:     map[string]adapters.Adapter{"fake": fakeAdapter{}},
	})
	if err != nil {
		t.Fatalf("runtime.New: %v", err)
	}
	t.Cleanup(rt.Close)
	return rt
}

// rpcStart arranca un job por RPC directo (como hace la CLI), sin pasar por
// una tool MCP: "start" ya no se expone como tool (ver NewServer), así que
// los tests que necesitan un job en curso lo arrancan por acá.
func rpcStart(t *testing.T, dial Dialer, req api.StartRequest) api.StartResponse {
	t.Helper()
	conn, err := dial()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var resp api.StartResponse
	if err := api.Call(conn, "start", req, &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func connectedClient(t *testing.T, dial Dialer) *sdk.ClientSession {
	t.Helper()
	return connectedClientWithNotify(t, dial, notify.Config{})
}

func connectedClientWithNotify(t *testing.T, dial Dialer, notifyCfg notify.Config) *sdk.ClientSession {
	t.Helper()
	server := NewServer(dial, notifyCfg)
	serverTransport, clientTransport := sdk.NewInMemoryTransports()

	ctx := context.Background()
	go func() {
		_, _ = server.Connect(ctx, serverTransport, nil)
	}()

	client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func callTool[T any](t *testing.T, cs *sdk.ClientSession, name string, args any) T {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	if res.IsError {
		var text string
		if len(res.Content) > 0 {
			if tc, ok := res.Content[0].(*sdk.TextContent); ok {
				text = tc.Text
			}
		}
		t.Fatalf("CallTool %s devolvió error: %s", name, text)
	}
	var out T
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMCPStatusAndResultObserveRPCStartedJob(t *testing.T) {
	repo := initTestRepo(t)
	rt := newTestRuntime(t)
	dial := rpcDialer(t, rt)
	cs := connectedClient(t, dial)

	// "start" ya no es una tool MCP (ver NewServer): el modelo delegador
	// solo ve "run", así que acá arrancamos el job por RPC directo, como
	// hace la CLI, y probamos que status/result lo observan igual.
	start := rpcStart(t, dial, api.StartRequest{Adapter: "fake", Repo: repo, Prompt: "hola", ReadOnly: true})
	if start.ID == "" {
		t.Fatal("start debe devolver un id")
	}

	deadline := time.Now().Add(5 * time.Second)
	var status api.StatusResponse
	for time.Now().Before(deadline) {
		status = callTool[api.StatusResponse](t, cs, "status", map[string]any{"id": start.ID})
		if status.State == "succeeded" || status.State == "failed" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if status.State != "succeeded" {
		t.Fatalf("estado inesperado: %+v", status)
	}

	result := callTool[api.ResultResponse](t, cs, "result", map[string]any{"id": start.ID})
	if !result.Available || result.Result != "ok" {
		t.Fatalf("resultado inesperado: %+v", result)
	}
}

func TestMCPResultExposesReasonForFailedRPCJob(t *testing.T) {
	repo := initTestRepo(t)
	rt, err := runtime.New(runtime.Config{
		StateDir:     t.TempDir(),
		WorktreeRoot: t.TempDir(),
		Adapters:     map[string]adapters.Adapter{"failing": failingAdapter{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	dial := rpcDialer(t, rt)
	cs := connectedClient(t, dial)

	result := callTool[api.ResultResponse](t, cs, "run", map[string]any{
		"adapter": "failing", "repo": repo, "prompt": "hola", "read_only": true, "timeout_seconds": 5,
	})
	if !result.Available || result.State != "failed" || result.Reason == "" {
		t.Fatalf("resultado fallido inesperado: %+v", result)
	}
}

func TestMCPResultPendingWhileNonTerminal(t *testing.T) {
	rt := newTestRuntime(t)
	dial := rpcDialer(t, rt)
	cs := connectedClient(t, dial)

	// Trabajo inexistente todavía: id inválido debe fallar validación.
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "status", Arguments: map[string]any{"id": ""}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("status sin id válido debería reportar error de validación")
	}
}

func TestMCPRunStartsAndWaitsInOneCall(t *testing.T) {
	repo := initTestRepo(t)
	rt := newTestRuntime(t)
	dial := rpcDialer(t, rt)
	cs := connectedClient(t, dial)

	result := callTool[api.ResultResponse](t, cs, "run", map[string]any{
		"adapter": "fake", "repo": repo, "prompt": "hola", "read_only": true, "timeout_seconds": 5,
	})
	if !result.Available || result.Result != "ok" {
		t.Fatalf("run debe devolver el resultado final en una sola llamada: %+v", result)
	}
}

func TestMCPWaitBlocksUntilTerminal(t *testing.T) {
	repo := initTestRepo(t)
	rt := newTestRuntime(t)
	dial := rpcDialer(t, rt)
	cs := connectedClient(t, dial)

	start := rpcStart(t, dial, api.StartRequest{Adapter: "fake", Repo: repo, Prompt: "hola", ReadOnly: true})

	result := callTool[api.ResultResponse](t, cs, "wait", map[string]any{"id": start.ID, "timeout_seconds": 5})
	if !result.Available || result.Result != "ok" {
		t.Fatalf("wait debe devolver el resultado terminal: %+v", result)
	}
}

func TestMCPWaitTimesOutWithoutBlockingForever(t *testing.T) {
	repo := initTestRepo(t)
	rt, err := newSlowRuntime(t, repo)
	if err != nil {
		t.Fatal(err)
	}
	dial := rpcDialer(t, rt)
	cs := connectedClient(t, dial)
	start := rpcStart(t, dial, api.StartRequest{Adapter: "slow", Repo: repo, Prompt: "hola", ReadOnly: true})

	begin := time.Now()
	result := callTool[api.ResultResponse](t, cs, "wait", map[string]any{"id": start.ID, "timeout_seconds": 1})
	if result.Available {
		t.Fatal("wait no debe devolver disponible antes de que el job termine")
	}
	if time.Since(begin) > 3*time.Second {
		t.Fatal("wait tardó demasiado en respetar el timeout")
	}
}

// slowAdapter simula un trabajo que nunca termina dentro del timeout de test.
type slowAdapter struct{}

func (slowAdapter) Name() string { return "slow" }
func (slowAdapter) Capabilities() adapters.Capabilities {
	return adapters.Capabilities{Cancel: false}
}
func (slowAdapter) Build(req adapters.StartRequest, worktreeDir string) (adapters.ProcessSpec, error) {
	return adapters.ProcessSpec{Path: "/bin/sh", Args: []string{"-c", "sleep 5"}, Dir: worktreeDir}, nil
}
func (slowAdapter) Result(raw []byte) ([]byte, error) { return raw, nil }

func newSlowRuntime(t *testing.T, repo string) (*runtime.Runtime, error) {
	t.Helper()
	rt, err := runtime.New(runtime.Config{
		StateDir:     t.TempDir(),
		WorktreeRoot: t.TempDir(),
		Workers:      1,
		Adapters:     map[string]adapters.Adapter{"slow": slowAdapter{}},
	})
	if err == nil {
		t.Cleanup(rt.Close)
	}
	return rt, err
}

func TestMCPCancelUnsupportedReturnsFalse(t *testing.T) {
	repo := initTestRepo(t)
	rt := newTestRuntime(t)
	dial := rpcDialer(t, rt)
	cs := connectedClient(t, dial)

	start := rpcStart(t, dial, api.StartRequest{Adapter: "fake", Repo: repo, Prompt: "hola", ReadOnly: true})
	cancel := callTool[api.CancelResponse](t, cs, "cancel", map[string]any{"id": start.ID})
	if cancel.Supported {
		t.Fatal("fakeAdapter no declara cancelación soportada")
	}
}
