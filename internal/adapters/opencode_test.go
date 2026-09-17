package adapters

import (
	"strings"
	"testing"
)

func TestOpenCodeAdapterDeclaresNoCancel(t *testing.T) {
	a := OpenCodeAdapter{}
	if a.Capabilities().Cancel {
		t.Fatal("opencode no debe declarar cancelación soportada")
	}
}

func TestOpenCodeAdapterBuildsReadOnlyFlags(t *testing.T) {
	withFakeLookPath(t, map[string]string{"opencode": "/usr/bin/opencode"})
	a := OpenCodeAdapter{}
	spec, err := a.Build(StartRequest{Prompt: "hola", ReadOnly: true}, "/tmp/wt")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := []string{"run", "--format", "json", "--pure", "--dir", "/tmp/wt"}
	if len(spec.Args) != len(want) {
		t.Fatalf("argv=%v, esperado %v", spec.Args, want)
	}
	for i, w := range want {
		if spec.Args[i] != w {
			t.Fatalf("argv[%d]=%q, esperado %q (args=%v)", i, spec.Args[i], w, spec.Args)
		}
	}
	for _, arg := range spec.Args {
		if arg == "--auto" {
			t.Fatal("modo solo lectura no debe agregar --auto")
		}
		if strings.Contains(arg, "hola") {
			t.Fatal("el prompt no debe aparecer en argv")
		}
	}
	if string(spec.Stdin) != "hola" {
		t.Fatalf("el prompt debe viajar por stdin, got %q", spec.Stdin)
	}
}

func TestOpenCodeAdapterWriteModeAddsAuto(t *testing.T) {
	withFakeLookPath(t, map[string]string{"opencode": "/usr/bin/opencode"})
	a := OpenCodeAdapter{}
	spec, err := a.Build(StartRequest{Prompt: "hola", ReadOnly: false}, "/tmp/wt")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	found := false
	for _, arg := range spec.Args {
		if arg == "--auto" {
			found = true
		}
	}
	if !found {
		t.Fatal("modo escritura debe agregar --auto")
	}
}

func TestOpenCodeAdapterReadOnlySetsDenyEnv(t *testing.T) {
	withFakeLookPath(t, map[string]string{"opencode": "/usr/bin/opencode"})
	a := OpenCodeAdapter{}
	spec, err := a.Build(StartRequest{Prompt: "hola", ReadOnly: true}, "/tmp/wt")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	found := false
	for _, e := range spec.Env {
		if strings.HasPrefix(e, "OPENCODE_CONFIG_CONTENT=") &&
			strings.Contains(e, `"edit":"deny"`) &&
			strings.Contains(e, `"write":"deny"`) &&
			strings.Contains(e, `"bash":"deny"`) &&
			strings.Contains(e, `"webfetch":"deny"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("modo solo lectura debe declarar OPENCODE_CONFIG_CONTENT con permisos deny, got env=%v", spec.Env)
	}
}

func TestOpenCodeAdapterWriteModeHasNoDenyEnv(t *testing.T) {
	withFakeLookPath(t, map[string]string{"opencode": "/usr/bin/opencode"})
	a := OpenCodeAdapter{}
	spec, err := a.Build(StartRequest{Prompt: "hola", ReadOnly: false}, "/tmp/wt")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, e := range spec.Env {
		if strings.HasPrefix(e, "OPENCODE_CONFIG_CONTENT=") {
			t.Fatalf("modo escritura no debe declarar OPENCODE_CONFIG_CONTENT, got env=%v", spec.Env)
		}
	}
}

func TestOpenCodeAdapterOmitsModelAndEffortWhenEmpty(t *testing.T) {
	withFakeLookPath(t, map[string]string{"opencode": "/usr/bin/opencode"})
	a := OpenCodeAdapter{}
	spec, err := a.Build(StartRequest{Prompt: "hola", ReadOnly: true}, "/tmp/wt")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, arg := range spec.Args {
		if arg == "-m" || arg == "--variant" {
			t.Fatalf("modelo/esfuerzo vacíos no deben agregar flags, got %v", spec.Args)
		}
	}
}

func TestOpenCodeAdapterAddsModelAndEffortFlags(t *testing.T) {
	withFakeLookPath(t, map[string]string{"opencode": "/usr/bin/opencode"})
	a := OpenCodeAdapter{}
	spec, err := a.Build(StartRequest{Prompt: "hola", ReadOnly: true, Model: "opencode/mimo-v2.5-free", Effort: "high"}, "/tmp/wt")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	foundModel, foundEffort := "", ""
	for i, arg := range spec.Args {
		if arg == "-m" && i+1 < len(spec.Args) {
			foundModel = spec.Args[i+1]
		}
		if arg == "--variant" && i+1 < len(spec.Args) {
			foundEffort = spec.Args[i+1]
		}
	}
	if foundModel != "opencode/mimo-v2.5-free" || foundEffort != "high" {
		t.Fatalf("flags de modelo/esfuerzo inesperados: model=%q effort=%q (args=%v)", foundModel, foundEffort, spec.Args)
	}
}

func TestOpenCodeAdapterMissingBinary(t *testing.T) {
	withFakeLookPath(t, map[string]string{})
	a := OpenCodeAdapter{}
	_, err := a.Build(StartRequest{ReadOnly: true}, "/tmp/wt")
	if err != ErrAdapterUnavailable {
		t.Fatalf("esperado ErrAdapterUnavailable, got %v", err)
	}
}

func TestOpenCodeResultConcatenatesTextParts(t *testing.T) {
	a := OpenCodeAdapter{}
	raw := strings.Join([]string{
		`{"type":"step_start","timestamp":1,"sessionID":"ses_1","part":{}}`,
		`{"type":"text","timestamp":2,"sessionID":"ses_1","part":{"id":"prt_1","type":"text","text":"OPENCODE-OK","time":{}}}`,
		`{"type":"step_finish","timestamp":3,"part":{"reason":"stop"}}`,
	}, "\n")
	out, err := a.Result([]byte(raw))
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if string(out) != "OPENCODE-OK" {
		t.Fatalf("respuesta inesperada: %q", out)
	}
}

func TestOpenCodeResultIgnoresGarbageLines(t *testing.T) {
	a := OpenCodeAdapter{}
	raw := strings.Join([]string{
		`not json`,
		`{"type":"step_start","part":{}}`,
		``,
		`{"type":"text","part":{"type":"text","text":"ok final"}}`,
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

func TestOpenCodeResultFallsBackToRawWithoutTextEvents(t *testing.T) {
	a := OpenCodeAdapter{}
	raw := `{"type":"step_start","part":{}}`
	out, err := a.Result([]byte(raw))
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if string(out) != raw {
		t.Fatalf("esperado fallback a la salida cruda, got %q", out)
	}
}
