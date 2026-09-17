package adapters

import (
	"strings"
	"testing"
)

func TestCodexAdapterDeclaresNoCancel(t *testing.T) {
	a := CodexAdapter{}
	if a.Capabilities().Cancel {
		t.Fatal("codex no debe declarar cancelación soportada")
	}
}

func TestCodexAdapterBuildsReadOnlyFlags(t *testing.T) {
	withFakeLookPath(t, map[string]string{"codex": "/usr/bin/codex"})
	a := CodexAdapter{}
	spec, err := a.Build(StartRequest{Prompt: "hola", ReadOnly: true}, "/tmp/wt")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := []string{"exec", "--json", "--skip-git-repo-check", "-C", "/tmp/wt", "--sandbox", "read-only", "-"}
	if len(spec.Args) != len(want) {
		t.Fatalf("argv=%v, esperado %v", spec.Args, want)
	}
	for i, w := range want {
		if spec.Args[i] != w {
			t.Fatalf("argv[%d]=%q, esperado %q (args=%v)", i, spec.Args[i], w, spec.Args)
		}
	}
	for _, arg := range spec.Args {
		if strings.Contains(arg, "hola") {
			t.Fatal("el prompt no debe aparecer en argv")
		}
	}
	if string(spec.Stdin) != "hola" {
		t.Fatalf("el prompt debe viajar por stdin, got %q", spec.Stdin)
	}
}

func TestCodexAdapterBuildsWriteModeSandbox(t *testing.T) {
	withFakeLookPath(t, map[string]string{"codex": "/usr/bin/codex"})
	a := CodexAdapter{}
	spec, err := a.Build(StartRequest{Prompt: "hola", ReadOnly: false}, "/tmp/wt")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	found := false
	for i, arg := range spec.Args {
		if arg == "--sandbox" && i+1 < len(spec.Args) {
			if spec.Args[i+1] != "workspace-write" {
				t.Fatalf("modo escritura debe usar --sandbox workspace-write, got %q", spec.Args[i+1])
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no se encontró --sandbox en argv")
	}
}

func TestCodexAdapterOmitsModelAndEffortWhenEmpty(t *testing.T) {
	withFakeLookPath(t, map[string]string{"codex": "/usr/bin/codex"})
	a := CodexAdapter{}
	spec, err := a.Build(StartRequest{Prompt: "hola", ReadOnly: true}, "/tmp/wt")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, arg := range spec.Args {
		if arg == "-m" || arg == "-c" {
			t.Fatalf("modelo/esfuerzo vacíos no deben agregar flags, got %v", spec.Args)
		}
	}
}

func TestCodexAdapterAddsModelAndEffortFlags(t *testing.T) {
	withFakeLookPath(t, map[string]string{"codex": "/usr/bin/codex"})
	a := CodexAdapter{}
	spec, err := a.Build(StartRequest{Prompt: "hola", ReadOnly: true, Model: "gpt-5-codex", Effort: "high"}, "/tmp/wt")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	foundModel, foundEffort := "", ""
	for i, arg := range spec.Args {
		if arg == "-m" && i+1 < len(spec.Args) {
			foundModel = spec.Args[i+1]
		}
		if arg == "-c" && i+1 < len(spec.Args) {
			foundEffort = spec.Args[i+1]
		}
	}
	if foundModel != "gpt-5-codex" {
		t.Fatalf("modelo inesperado: %q (args=%v)", foundModel, spec.Args)
	}
	if foundEffort != `model_reasoning_effort="high"` {
		t.Fatalf("override de esfuerzo inesperado: %q (args=%v)", foundEffort, spec.Args)
	}
}

func TestCodexAdapterMissingBinary(t *testing.T) {
	withFakeLookPath(t, map[string]string{})
	a := CodexAdapter{}
	_, err := a.Build(StartRequest{ReadOnly: true}, "/tmp/wt")
	if err != ErrAdapterUnavailable {
		t.Fatalf("esperado ErrAdapterUnavailable, got %v", err)
	}
}

func TestCodexResultReturnsLastAgentMessage(t *testing.T) {
	a := CodexAdapter{}
	raw := strings.Join([]string{
		`{"type":"thread.started","thread_id":"01a0"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"CODEX-OK"}}`,
		`{"type":"turn.completed","usage":{}}`,
	}, "\n")
	out, err := a.Result([]byte(raw))
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if string(out) != "CODEX-OK" {
		t.Fatalf("respuesta inesperada: %q", out)
	}
}

func TestCodexResultIgnoresGarbageLines(t *testing.T) {
	a := CodexAdapter{}
	raw := strings.Join([]string{
		`not json`,
		`{"type":"thread.started","thread_id":"01a0"}`,
		``,
		`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"ok final"}}`,
		`trailing noise`,
	}, "\n")
	out, err := a.Result([]byte(raw))
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if string(out) != "ok final" {
		t.Fatalf("respuesta inesperada: %q", out)
	}
}

// Un item de error informativo (por ejemplo, presupuesto de skills) puede
// aparecer sin que el trabajo haya fallado; el agent_message posterior sigue
// siendo la respuesta.
func TestCodexResultDoesNotTreatErrorItemAsFailure(t *testing.T) {
	a := CodexAdapter{}
	raw := strings.Join([]string{
		`{"type":"thread.started","thread_id":"01a0"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"id":"item_0","type":"error","message":"presupuesto de skills agotado"}}`,
		`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"CODEX-OK"}}`,
		`{"type":"turn.completed","usage":{}}`,
	}, "\n")
	out, err := a.Result([]byte(raw))
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if string(out) != "CODEX-OK" {
		t.Fatalf("respuesta inesperada: %q", out)
	}
}

func TestCodexResultFallsBackToRawWithoutAgentMessage(t *testing.T) {
	a := CodexAdapter{}
	raw := `{"type":"thread.started","thread_id":"01a0"}`
	out, err := a.Result([]byte(raw))
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if string(out) != raw {
		t.Fatalf("esperado fallback a la salida cruda, got %q", out)
	}
}
