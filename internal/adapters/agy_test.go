package adapters

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAgyAdapterDeclaresNoCancel(t *testing.T) {
	a := AgyAdapter{}
	if a.Capabilities().Cancel {
		t.Fatal("agy no debe declarar cancelación soportada")
	}
}

func TestAgyAdapterBuildsStreamJSONFlags(t *testing.T) {
	withFakeLookPath(t, map[string]string{"agy": "/usr/bin/agy"})
	a := AgyAdapter{}
	spec, err := a.Build(StartRequest{Prompt: "hola", ReadOnly: true}, "/tmp/wt")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := []string{"--print=", "--input-format", "stream-json", "--output-format", "stream-json", "--sandbox"}
	if len(spec.Args) < len(want) {
		t.Fatalf("argv incompleto: %v", spec.Args)
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
}

func TestAgyAdapterPromptTravelsByStdinAsNDJSON(t *testing.T) {
	withFakeLookPath(t, map[string]string{"agy": "/usr/bin/agy"})
	a := AgyAdapter{}
	spec, err := a.Build(StartRequest{Prompt: "hola", ReadOnly: false}, "/tmp/wt")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var ev agyUserEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(spec.Stdin))), &ev); err != nil {
		t.Fatalf("stdin no es la línea NDJSON esperada: %v (stdin=%q)", err, spec.Stdin)
	}
	if ev.Event != "user" || ev.Message.Role != "user" || ev.Message.Content != "hola" {
		t.Fatalf("evento de usuario inesperado: %+v", ev)
	}
}

func TestAgyAdapterReadOnlyPrependsToolRestriction(t *testing.T) {
	withFakeLookPath(t, map[string]string{"agy": "/usr/bin/agy"})
	a := AgyAdapter{}
	spec, err := a.Build(StartRequest{Prompt: "hola", ReadOnly: true}, "/tmp/wt")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var ev agyUserEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(spec.Stdin))), &ev); err != nil {
		t.Fatalf("stdin inválido: %v", err)
	}
	if !strings.Contains(ev.Message.Content, "run_command") || !strings.HasSuffix(ev.Message.Content, "hola") {
		t.Fatalf("modo solo lectura debe anteponer la restricción de herramientas, got %q", ev.Message.Content)
	}
	for _, arg := range spec.Args {
		if arg == "--dangerously-skip-permissions" {
			t.Fatal("modo solo lectura no debe usar --dangerously-skip-permissions")
		}
	}
}

func TestAgyAdapterWriteModeSkipsRestrictionAndAddsSkipPermissions(t *testing.T) {
	withFakeLookPath(t, map[string]string{"agy": "/usr/bin/agy"})
	a := AgyAdapter{}
	spec, err := a.Build(StartRequest{Prompt: "hola", ReadOnly: false}, "/tmp/wt")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var ev agyUserEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(spec.Stdin))), &ev); err != nil {
		t.Fatalf("stdin inválido: %v", err)
	}
	if ev.Message.Content != "hola" {
		t.Fatalf("modo escritura no debe anteponer restricción, got %q", ev.Message.Content)
	}
	found := false
	for _, arg := range spec.Args {
		if arg == "--dangerously-skip-permissions" {
			found = true
		}
	}
	if !found {
		t.Fatal("modo escritura debe agregar --dangerously-skip-permissions")
	}
}

func TestAgyAdapterOmitsModelAndEffortWhenEmpty(t *testing.T) {
	withFakeLookPath(t, map[string]string{"agy": "/usr/bin/agy"})
	a := AgyAdapter{}
	spec, err := a.Build(StartRequest{Prompt: "hola", ReadOnly: true}, "/tmp/wt")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, arg := range spec.Args {
		if arg == "--model" || arg == "--effort" {
			t.Fatalf("modelo/esfuerzo vacíos no deben agregar flags, got %v", spec.Args)
		}
	}
}

func TestAgyAdapterAddsModelAndEffortFlags(t *testing.T) {
	withFakeLookPath(t, map[string]string{"agy": "/usr/bin/agy"})
	a := AgyAdapter{}
	spec, err := a.Build(StartRequest{Prompt: "hola", ReadOnly: true, Model: "gemini-3.8-flash-low", Effort: "medium"}, "/tmp/wt")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	found := map[string]string{}
	for i, arg := range spec.Args {
		if (arg == "--model" || arg == "--effort") && i+1 < len(spec.Args) {
			found[arg] = spec.Args[i+1]
		}
	}
	if found["--model"] != "gemini-3.8-flash-low" || found["--effort"] != "medium" {
		t.Fatalf("flags de modelo/esfuerzo inesperados: %v (args=%v)", found, spec.Args)
	}
}

func TestAgyAdapterMissingBinary(t *testing.T) {
	withFakeLookPath(t, map[string]string{})
	a := AgyAdapter{}
	_, err := a.Build(StartRequest{ReadOnly: true}, "/tmp/wt")
	if err != ErrAdapterUnavailable {
		t.Fatalf("esperado ErrAdapterUnavailable, got %v", err)
	}
}

func TestAgyResultParsesSuccessFromNDJSON(t *testing.T) {
	a := AgyAdapter{}
	raw := strings.Join([]string{
		`{"event":"init","conversation_id":"c1","init":{"cwd":"/tmp/wt","tools":["view_file"],"permission_mode":"request-review"}}`,
		`{"event":"step_update","step_update":{"step_index":0,"state":"DONE","step_type":"agent_response","text_delta":"listo"}}`,
		`{"event":"result","result":{"conversation_id":"c1","status":"SUCCESS","response":"todo listo","error":null,"duration_seconds":1.2}}`,
	}, "\n")
	out, err := a.Result([]byte(raw))
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if string(out) != "todo listo" {
		t.Fatalf("respuesta inesperada: %q", out)
	}
}

func TestAgyResultReportsStructuredError(t *testing.T) {
	a := AgyAdapter{}
	raw := `{"event":"result","result":{"conversation_id":"c1","status":"ERROR","response":"","error":"algo se rompió"}}`
	_, err := a.Result([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "algo se rompió") {
		t.Fatalf("esperado error con el mensaje estructurado, got %v", err)
	}
}

func TestAgyResultDetectsToolPermissionDenial(t *testing.T) {
	a := AgyAdapter{}
	raw := strings.Join([]string{
		`{"event":"step_update","step_update":{"step_index":1,"state":"ERROR","step_type":"tool","tool_name":"run_command","tool_info":{"error":{"type":"TOOL_ERROR","message":"permission required"}}}}`,
		`{"event":"result","result":{"conversation_id":"c1","status":"SUCCESS","response":"","error":null}}`,
	}, "\n")
	if _, err := a.Result([]byte(raw)); err != ErrAgyPermissionDenied {
		t.Fatalf("esperado ErrAgyPermissionDenied, got %v", err)
	}
}

// Un fallo de tool que no es de permisos (visto en vivo: el servidor de
// sandbox de agy corta la conexión) no debe reportarse como falta de permisos,
// porque manda a quien diagnostica a editar settings.json sin motivo.
func TestAgyResultDoesNotBlamePermissionsForUnrelatedToolError(t *testing.T) {
	a := AgyAdapter{}
	raw := strings.Join([]string{
		`{"event":"step_update","step_update":{"step_index":1,"state":"ERROR","step_type":"tool","tool_name":"run_command","tool_info":{"error":{"type":"TOOL_ERROR","message":"connecting to sandbox server: read unix @->@: recvmsg: connection reset by peer"}}}}`,
		`{"event":"result","result":{"conversation_id":"c1","status":"SUCCESS","response":"","error":null}}`,
	}, "\n")
	if _, err := a.Result([]byte(raw)); err == ErrAgyPermissionDenied {
		t.Fatal("un error de sandbox no es una denegación de permisos")
	}
}

func TestAgyResultDetectsLegacyPermissionDenialText(t *testing.T) {
	a := AgyAdapter{}
	msg := `jetski: no output produced — a tool required the "read_file" permission that headless mode cannot prompt for, so it was auto-denied.`
	if _, err := a.Result([]byte(msg)); err != ErrAgyPermissionDenied {
		t.Fatalf("Result debe detectar denegación de permisos, got err=%v", err)
	}
}

func TestAgyResultIgnoresGarbageLinesInterleaved(t *testing.T) {
	a := AgyAdapter{}
	raw := strings.Join([]string{
		`this is not json at all`,
		`{"event":"init","conversation_id":"c1","init":{"cwd":"/tmp/wt","tools":[],"permission_mode":"request-review"}}`,
		``,
		`   `,
		`{"event":"result","result":{"conversation_id":"c1","status":"SUCCESS","response":"ok final","error":null}}`,
		`trailing noise on stderr`,
	}, "\n")
	out, err := a.Result([]byte(raw))
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if string(out) != "ok final" {
		t.Fatalf("respuesta inesperada: %q", out)
	}
}
