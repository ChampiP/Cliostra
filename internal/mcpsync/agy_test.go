package mcpsync

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("no se pudo escribir %s: %v", path, err)
	}
	return path
}

func TestMissingEntries_UnaFaltante(t *testing.T) {
	dir := t.TempDir()
	desktop := writeConfig(t, dir, "desktop.json", `{"mcpServers":{"context7":{"serverUrl":"https://mcp.context7.com/mcp"},"engram":{"command":"engram","args":["mcp"]}}}`)
	headless := writeConfig(t, dir, "headless.json", `{"mcpServers":{"engram":{"command":"engram","args":["mcp"]}}}`)

	missing, err := missingEntries(desktop, headless)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(missing) != 1 {
		t.Fatalf("esperaba 1 faltante, obtuve %d: %v", len(missing), missing)
	}
	if _, ok := missing["context7"]; !ok {
		t.Fatalf("esperaba que context7 esté marcado como faltante: %v", missing)
	}
}

func TestMissingEntries_HeadlessInexistente(t *testing.T) {
	dir := t.TempDir()
	desktop := writeConfig(t, dir, "desktop.json", `{"mcpServers":{"context7":{"serverUrl":"https://mcp.context7.com/mcp"},"engram":{"command":"engram","args":["mcp"]}}}`)
	headless := filepath.Join(dir, "no-existe.json")

	missing, err := missingEntries(desktop, headless)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(missing) != 2 {
		t.Fatalf("esperaba 2 faltantes, obtuve %d: %v", len(missing), missing)
	}
}

func TestMissingEntries_DesktopInexistente(t *testing.T) {
	dir := t.TempDir()
	desktop := filepath.Join(dir, "no-existe.json")
	headless := writeConfig(t, dir, "headless.json", `{"mcpServers":{}}`)

	missing, err := missingEntries(desktop, headless)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("esperaba 0 faltantes, obtuve %d: %v", len(missing), missing)
	}
}

func TestAddArgs_ServerURL(t *testing.T) {
	args := addArgs("context7", mcpServerEntry{ServerURL: "https://mcp.context7.com/mcp"})
	want := []string{"mcp", "add", "context7", "https://mcp.context7.com/mcp"}
	if !equalSlices(args, want) {
		t.Fatalf("args = %v, esperaba %v", args, want)
	}
}

func TestAddArgs_Command(t *testing.T) {
	args := addArgs("engram", mcpServerEntry{Command: "engram", Args: []string{"mcp"}})
	want := []string{"mcp", "add", "engram", "engram", "mcp"}
	if !equalSlices(args, want) {
		t.Fatalf("args = %v, esperaba %v", args, want)
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// fakeExecCommand permite simular `agy mcp add` sin ejecutar el binario
// real: usa el propio binario de test relanzado en modo "helper" (patrón
// estándar de exec.Command en la stdlib de Go).
func fakeExecCommand(fail map[string]bool) func(string, ...string) *exec.Cmd {
	return func(name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", name}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		serverName := ""
		if len(args) >= 3 {
			serverName = args[2]
		}
		if fail[serverName] {
			cmd.Env = append(cmd.Env, "GO_HELPER_SHOULD_FAIL=1")
		}
		return cmd
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	if os.Getenv("GO_HELPER_SHOULD_FAIL") == "1" {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestSyncAgyPaths_InvocaAddParaCadaFaltante(t *testing.T) {
	dir := t.TempDir()
	desktop := writeConfig(t, dir, "desktop.json", `{"mcpServers":{"context7":{"serverUrl":"https://mcp.context7.com/mcp"},"engram":{"command":"engram","args":["mcp"]}}}`)
	headless := writeConfig(t, dir, "headless.json", `{"mcpServers":{}}`)

	origLookPath, origExec := lookPath, execCommand
	defer func() { lookPath, execCommand = origLookPath, origExec }()
	lookPath = func(string) (string, error) { return "/usr/bin/agy", nil }
	execCommand = fakeExecCommand(nil)

	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	if err := syncAgyPaths(logger, desktop, headless); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	cfg, err := readMcpConfig(headless)
	_ = cfg
	if err != nil {
		t.Fatalf("no se pudo releer headless: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("context7")) || !bytes.Contains(buf.Bytes(), []byte("engram")) {
		t.Fatalf("esperaba log de ambos servidores copiados, obtuve: %s", buf.String())
	}
}

func TestSyncAgyPaths_SigueTrasFalloEnUno(t *testing.T) {
	dir := t.TempDir()
	desktop := writeConfig(t, dir, "desktop.json", `{"mcpServers":{"context7":{"serverUrl":"https://mcp.context7.com/mcp"},"engram":{"command":"engram","args":["mcp"]}}}`)
	headless := writeConfig(t, dir, "headless.json", `{"mcpServers":{}}`)

	origLookPath, origExec := lookPath, execCommand
	defer func() { lookPath, execCommand = origLookPath, origExec }()
	lookPath = func(string) (string, error) { return "/usr/bin/agy", nil }
	execCommand = fakeExecCommand(map[string]bool{"context7": true})

	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	if err := syncAgyPaths(logger, desktop, headless); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("no se pudo copiar")) {
		t.Fatalf("esperaba log de fallo para context7, obtuve: %s", buf.String())
	}
	if !bytes.Contains(buf.Bytes(), []byte("engram")) {
		t.Fatalf("esperaba que engram se procese igual pese al fallo de context7, obtuve: %s", buf.String())
	}
}

func TestSyncAgyPaths_AgyNoInstalado(t *testing.T) {
	dir := t.TempDir()
	desktop := writeConfig(t, dir, "desktop.json", `{"mcpServers":{"context7":{"serverUrl":"https://mcp.context7.com/mcp"}}}`)
	headless := writeConfig(t, dir, "headless.json", `{"mcpServers":{}}`)

	origLookPath, origExec := lookPath, execCommand
	defer func() { lookPath, execCommand = origLookPath, origExec }()
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	calledExec := false
	execCommand = func(name string, args ...string) *exec.Cmd {
		calledExec = true
		return exec.Command("true")
	}

	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	if err := syncAgyPaths(logger, desktop, headless); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if calledExec {
		t.Fatalf("no debería haberse invocado exec.Command sin agy instalado")
	}
}
