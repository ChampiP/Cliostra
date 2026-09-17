package adapters

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// AgyAdapter ejecuta Google Antigravity (`agy`) usando su protocolo NDJSON
// (stream-json): el prompt viaja por stdin como un evento "user" y la
// salida se interpreta evento por evento, en vez de adivinar el resultado a
// partir de texto plano.
type AgyAdapter struct{}

func (AgyAdapter) Name() string { return "agy" }

func (AgyAdapter) Capabilities() Capabilities {
	return Capabilities{Cancel: false}
}

// agyReadOnlyPrefix se antepone al prompt del usuario solo en modo solo
// lectura: en headless, la herramienta run_command exige un permiso que
// nunca está concedido, y si el agente la intenta la corrida entera aborta.
// call_mcp_tool sí está permitida: da acceso a servidores MCP de solo
// consulta (memoria, grafo de código) sin ejecutar comandos de shell.
const agyReadOnlyPrefix = `RESTRICCIÓN DE HERRAMIENTAS: en este entorno la herramienta run_command está bloqueada y siempre falla. NO la uses bajo ninguna circunstancia. Usá exclusivamente: list_dir, view_file, grep_search, find_by_name, call_mcp_tool.

`

// agyUserEvent es la línea NDJSON de entrada que agy espera por stdin en
// modo --input-format stream-json.
type agyUserEvent struct {
	Event   string `json:"event"`
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
}

// Build arma argv fijo para agy en modo protocolar NDJSON. El prompt no va
// en argv (nunca queda expuesto en `ps`): viaja por stdin como una línea
// {"event":"user",...}. read_only=true antepone una restricción de
// herramientas al prompt; read_only=false agrega
// --dangerously-skip-permissions, la única forma de que agy edite/ejecute de
// verdad en headless. El worktree administrado sigue siendo el límite: agy
// corre con Dir=worktreeDir, nunca en el repo real.
func (AgyAdapter) Build(req StartRequest, worktreeDir string) (ProcessSpec, error) {
	path, err := lookPath("agy")
	if err != nil {
		return ProcessSpec{}, ErrAdapterUnavailable
	}

	prompt := req.Prompt
	if req.ReadOnly {
		prompt = agyReadOnlyPrefix + prompt
	}

	var ev agyUserEvent
	ev.Event = "user"
	ev.Message.Role = "user"
	ev.Message.Content = prompt
	line, err := json.Marshal(ev)
	if err != nil {
		return ProcessSpec{}, err
	}
	stdin := append(line, '\n')

	args := []string{"--print=", "--input-format", "stream-json", "--output-format", "stream-json", "--sandbox"}
	if !req.ReadOnly {
		args = append(args, "--dangerously-skip-permissions")
	}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.Effort != "" {
		args = append(args, "--effort", req.Effort)
	}
	return ProcessSpec{
		Path:  path,
		Args:  args,
		Dir:   worktreeDir,
		Stdin: stdin,
	}, nil
}

// ErrAgyPermissionDenied indica que agy no ejecutó (o no completó) el
// trabajo porque una herramienta requería un permiso que headless no puede
// conceder.
var ErrAgyPermissionDenied = errors.New("agy denegó una herramienta en modo headless (falta allow-rule en settings.json)")

// agyEvent es una línea NDJSON de salida genérica; solo "result" y
// "step_update" importan para interpretar el resultado.
type agyEvent struct {
	Event      string         `json:"event"`
	Result     *agyResult     `json:"result"`
	StepUpdate *agyStepUpdate `json:"step_update"`
}

type agyResult struct {
	Status   string  `json:"status"`
	Response string  `json:"response"`
	Error    *string `json:"error"`
}

type agyStepUpdate struct {
	StepType string       `json:"step_type"`
	State    string       `json:"state"`
	ToolInfo *agyToolInfo `json:"tool_info"`
}

type agyToolInfo struct {
	Error *agyToolError `json:"error"`
}

type agyToolError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// isPermissionError distingue un fallo de tool por falta de permisos de
// cualquier otro (por ejemplo, el servidor de sandbox de agy cortando la
// conexión). Confundirlos manda a quien diagnostica a editar settings.json
// sin motivo.
func isPermissionError(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "permission") || strings.Contains(lower, "auto-denied")
}

// Result recorre el NDJSON de salida de agy, se queda con el evento "result"
// y devuelve su "response". Líneas que no son JSON válido (agy a veces
// escupe texto suelto en stdout/stderr) se ignoran, salvo que contengan la
// señal de denegación de permisos ya conocida. Un step_update de una tool que
// terminó en ERROR por permisos también cuenta como esa señal.
func (AgyAdapter) Result(raw []byte) ([]byte, error) {
	var final *agyResult
	permissionDenied := false

	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev agyEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			if strings.Contains(string(line), "no output produced") && strings.Contains(string(line), "auto-denied") {
				permissionDenied = true
			}
			continue
		}
		switch ev.Event {
		case "result":
			final = ev.Result
		case "step_update":
			if ev.StepUpdate != nil && ev.StepUpdate.StepType == "tool" &&
				ev.StepUpdate.State == "ERROR" && ev.StepUpdate.ToolInfo != nil &&
				ev.StepUpdate.ToolInfo.Error != nil &&
				isPermissionError(ev.StepUpdate.ToolInfo.Error.Message) {
				permissionDenied = true
			}
		}
	}

	if final != nil {
		if final.Status == "ERROR" {
			msg := "error desconocido"
			if final.Error != nil && *final.Error != "" {
				msg = *final.Error
			}
			return nil, fmt.Errorf("agy reportó error: %s", msg)
		}
		if final.Response == "" && permissionDenied {
			return nil, ErrAgyPermissionDenied
		}
		return []byte(final.Response), nil
	}

	if permissionDenied {
		return nil, ErrAgyPermissionDenied
	}
	return raw, nil
}
