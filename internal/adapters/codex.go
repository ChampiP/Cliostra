package adapters

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
)

// CodexAdapter ejecuta `codex exec` (Codex CLI). El prompt viaja por stdin
// (argv termina en "-") para que nunca quede expuesto en `ps`, y la salida se
// interpreta como JSONL vía --json.
type CodexAdapter struct{}

func (CodexAdapter) Name() string { return "codex" }

func (CodexAdapter) Capabilities() Capabilities {
	return Capabilities{Cancel: true, Resume: true}
}

// Build arma argv fijo para `codex exec`. read_only=true usa
// --sandbox read-only; read_only=false usa --sandbox workspace-write, que ya
// es nativo y no requiere flags de "skip permisos". El esfuerzo no tiene flag
// propio: se pasa como override de config TOML (model_reasoning_effort).
// Si se pasa req.SessionID, reanuda la conversación existente vía `codex exec resume`.
func (CodexAdapter) Build(req StartRequest, worktreeDir string) (ProcessSpec, error) {
	path, err := lookPath("codex")
	if err != nil {
		return ProcessSpec{}, ErrAdapterUnavailable
	}

	sandbox := "workspace-write"
	if req.ReadOnly {
		sandbox = "read-only"
	}

	var args []string
	if req.SessionID != "" {
		args = []string{"exec", "resume", "--json", "--skip-git-repo-check"}
		if req.Model != "" {
			args = append(args, "-m", req.Model)
		}
		if req.Effort != "" {
			args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", req.Effort))
		}
		args = append(args, req.SessionID, "-")
	} else {
		args = []string{"exec", "--json", "--skip-git-repo-check", "-C", worktreeDir, "--sandbox", sandbox}
		if req.Model != "" {
			args = append(args, "-m", req.Model)
		}
		if req.Effort != "" {
			args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", req.Effort))
		}
		args = append(args, "-")
	}

	return ProcessSpec{
		Path:  path,
		Args:  args,
		Dir:   worktreeDir,
		Stdin: []byte(req.Prompt),
	}, nil
}

// codexEvent es una línea JSONL de salida de `codex exec --json`; solo
// item.completed importa para extraer el resultado.
type codexEvent struct {
	Type     string     `json:"type"`
	ThreadID string     `json:"thread_id,omitempty"`
	Item     *codexItem `json:"item"`
}

type codexItem struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	ThreadID string `json:"thread_id,omitempty"`
}

// Result recorre el JSONL de codex y devuelve el texto del último
// item.completed de tipo "agent_message". Items de tipo "error" pueden
// aparecer sin que el trabajo haya fallado (por ejemplo, un aviso de
// presupuesto de skills), así que no se tratan como fallo. Líneas que no
// parsean como JSON se ignoran. Sin ningún agent_message, devuelve la salida
// cruda tal cual.
func (CodexAdapter) Result(raw []byte) ([]byte, error) {
	var lastMessage string
	found := false

	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev codexEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.Type == "item.completed" && ev.Item != nil && ev.Item.Type == "agent_message" {
			lastMessage = ev.Item.Text
			found = true
		}
	}

	if found {
		return []byte(lastMessage), nil
	}
	return raw, nil
}

// ExtractSession extrae el thread_id de las líneas JSONL de codex si está presente.
func (CodexAdapter) ExtractSession(raw []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev codexEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.ThreadID != "" {
			return ev.ThreadID
		}
		if ev.Item != nil && ev.Item.ThreadID != "" {
			return ev.Item.ThreadID
		}
	}
	return ""
}
