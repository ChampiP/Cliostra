// Paquete integration contiene la prueba de humo de extremo a extremo:
// arranca el runtime en proceso sobre un socket Unix real, y ejercita
// start/status/result mediante el mismo cliente RPC que usan la CLI y el
// servidor MCP. No depende de Claude Code ni de `agy` reales.
package integration

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ChampiP/Cliostra/internal/adapters"
	"github.com/ChampiP/Cliostra/internal/api"
	"github.com/ChampiP/Cliostra/internal/runtime"
)

type fakeAdapter struct{}

func (fakeAdapter) Name() string { return "fake" }
func (fakeAdapter) Capabilities() adapters.Capabilities {
	return adapters.Capabilities{Cancel: false}
}
func (fakeAdapter) Build(req adapters.StartRequest, worktreeDir string) (adapters.ProcessSpec, error) {
	return adapters.ProcessSpec{Path: "/bin/sh", Args: []string{"-c", "cat >/dev/null; echo listo"}, Dir: worktreeDir, Stdin: []byte(req.Prompt)}, nil
}
func (fakeAdapter) Result(raw []byte) ([]byte, error) {
	return []byte(strings.TrimSpace(string(raw))), nil
}

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

// TestEndToEndSmoke arranca runtime+socket Unix real en proceso, hace start
// con un adaptador falso, sondea status y obtiene result: prueba que
// api + runtime + adapters + socket se conectan de punta a punta.
func TestEndToEndSmoke(t *testing.T) {
	repo := initTestRepo(t)

	rt, err := runtime.New(runtime.Config{
		StateDir:     t.TempDir(),
		WorktreeRoot: t.TempDir(),
		Workers:      2,
		Adapters:     map[string]adapters.Adapter{"fake": fakeAdapter{}},
	})
	if err != nil {
		t.Fatalf("runtime.New: %v", err)
	}
	defer rt.Close()

	sockPath := filepath.Join(t.TempDir(), "cliostra.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	handlers := runtime.Handlers(rt)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = api.Serve(conn, handlers)
			}()
		}
	}()

	dial := func() net.Conn {
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			t.Fatalf("Dial: %v", err)
		}
		return conn
	}

	var start api.StartResponse
	c1 := dial()
	if err := api.Call(c1, "start", api.StartRequest{Adapter: "fake", Repo: repo, Prompt: "hola", ReadOnly: true}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	c1.Close()
	if start.ID == "" {
		t.Fatal("se esperaba un id de trabajo")
	}

	deadline := time.Now().Add(5 * time.Second)
	var status api.StatusResponse
	for time.Now().Before(deadline) {
		c2 := dial()
		if err := api.Call(c2, "status", api.StatusRequest{ID: start.ID}, &status); err != nil {
			t.Fatalf("status: %v", err)
		}
		c2.Close()
		if status.State == "succeeded" || status.State == "failed" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if status.State != "succeeded" {
		t.Fatalf("estado inesperado: %+v", status)
	}

	var result api.ResultResponse
	c3 := dial()
	if err := api.Call(c3, "result", api.ResultRequest{ID: start.ID}, &result); err != nil {
		t.Fatalf("result: %v", err)
	}
	c3.Close()
	if !result.Available || result.Result != "listo" {
		t.Fatalf("resultado inesperado: %+v", result)
	}
}
