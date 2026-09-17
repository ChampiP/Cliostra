package adapters

import (
	"strings"
	"testing"
)

func TestClaudeCodeAdapterCapabilities(t *testing.T) {
	a := ClaudeCodeAdapter{}
	if !a.Capabilities().Cancel {
		t.Fatal("claude-code debe declarar cancelación soportada")
	}
	if a.Name() != "claude-code" {
		t.Fatalf("nombre inesperado: %s", a.Name())
	}
}

func TestClaudeCodeAdapterBuildsFixedArgvAndStdinPrompt(t *testing.T) {
	withFakeLookPath(t, map[string]string{"claude": "/usr/bin/claude"})
	a := ClaudeCodeAdapter{}
	spec, err := a.Build(StartRequest{Repo: "/tmp/repo", Prompt: "revisa esto", ReadOnly: true}, "/tmp/worktree")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if spec.Path != "/usr/bin/claude" {
		t.Fatalf("path inesperado: %s", spec.Path)
	}
	if string(spec.Stdin) != "revisa esto" {
		t.Fatalf("el prompt debe viajar por stdin, got %q", spec.Stdin)
	}
	for _, arg := range spec.Args {
		if strings.Contains(arg, "revisa esto") {
			t.Fatal("el prompt no debe aparecer en argv")
		}
	}
	if spec.Dir != "/tmp/worktree" {
		t.Fatalf("dir inesperado: %s", spec.Dir)
	}
}

func TestClaudeCodeAdapterWriteModeUsesAcceptEdits(t *testing.T) {
	withFakeLookPath(t, map[string]string{"claude": "/usr/bin/claude"})
	a := ClaudeCodeAdapter{}
	spec, err := a.Build(StartRequest{Repo: "/tmp/repo", Prompt: "x", ReadOnly: false}, "/tmp/worktree")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	found := false
	for i, arg := range spec.Args {
		if arg == "acceptEdits" && i > 0 && spec.Args[i-1] == "--permission-mode" {
			found = true
		}
	}
	if !found {
		t.Fatalf("modo escritura debe usar --permission-mode acceptEdits, got %v", spec.Args)
	}
}

func TestClaudeCodeAdapterOmitsModelAndEffortWhenEmpty(t *testing.T) {
	withFakeLookPath(t, map[string]string{"claude": "/usr/bin/claude"})
	a := ClaudeCodeAdapter{}
	spec, err := a.Build(StartRequest{Prompt: "x", ReadOnly: true}, "/tmp/wt")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, arg := range spec.Args {
		if arg == "--model" || arg == "--effort" {
			t.Fatalf("modelo/esfuerzo vacíos no deben agregar flags, got %v", spec.Args)
		}
	}
}

func TestClaudeCodeAdapterAddsModelAndEffortFlags(t *testing.T) {
	withFakeLookPath(t, map[string]string{"claude": "/usr/bin/claude"})
	a := ClaudeCodeAdapter{}
	spec, err := a.Build(StartRequest{Prompt: "x", ReadOnly: true, Model: "sonnet", Effort: "high"}, "/tmp/wt")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	found := map[string]string{}
	for i, arg := range spec.Args {
		if (arg == "--model" || arg == "--effort") && i+1 < len(spec.Args) {
			found[arg] = spec.Args[i+1]
		}
	}
	if found["--model"] != "sonnet" || found["--effort"] != "high" {
		t.Fatalf("flags de modelo/esfuerzo inesperados: %v (args=%v)", found, spec.Args)
	}
}

func TestClaudeCodeAdapterMissingBinary(t *testing.T) {
	withFakeLookPath(t, map[string]string{})
	a := ClaudeCodeAdapter{}
	_, err := a.Build(StartRequest{ReadOnly: true}, "/tmp/worktree")
	if err != ErrAdapterUnavailable {
		t.Fatalf("esperado ErrAdapterUnavailable, got %v", err)
	}
}
