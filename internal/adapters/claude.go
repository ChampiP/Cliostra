package adapters

// ClaudeCodeAdapter ejecuta Claude Code, en modo de planificación (solo
// lectura) o de edición autónoma según req.ReadOnly. El runtime le pasa la
// raíz real del repositorio para que los cambios sean visibles en vivo.
type ClaudeCodeAdapter struct{}

func (ClaudeCodeAdapter) Name() string { return "claude-code" }

func (ClaudeCodeAdapter) Capabilities() Capabilities {
	return Capabilities{Cancel: true}
}

// Build arma argv fijo para Claude Code: impresión no interactiva y el
// prompt únicamente por stdin. read_only=true usa modo plan (nunca edita);
// read_only=false usa acceptEdits (edita y corre comandos sin pedir
// confirmación interactiva, algo obligatorio en modo headless).
func (ClaudeCodeAdapter) Build(req StartRequest, worktreeDir string) (ProcessSpec, error) {
	path, err := lookPath("claude")
	if err != nil {
		return ProcessSpec{}, ErrAdapterUnavailable
	}
	mode := "plan"
	if !req.ReadOnly {
		mode = "acceptEdits"
	}
	args := []string{"--print", "--permission-mode", mode}
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
		Stdin: []byte(req.Prompt),
	}, nil
}

func (ClaudeCodeAdapter) Result(raw []byte) ([]byte, error) {
	return raw, nil
}
