package adapters

import (
	"bufio"
	"bytes"
	"encoding/json"
)

// OpenCodeAdapter ejecuta `opencode run`. El prompt viaja por stdin (no hay
// mensaje posicional), y --pure evita que cargue plugins externos, en
// particular el propio plugin de Cliostra instalado en
// ~/.config/opencode/plugin/, que podría delegar recursivamente.
type OpenCodeAdapter struct{}

func (OpenCodeAdapter) Name() string { return "opencode" }

// Cancel no fue verificada empíricamente contra el proceso real de opencode;
// se declara false para no prometer una capacidad no comprobada (mismo
// criterio que agy.go).
func (OpenCodeAdapter) Capabilities() Capabilities {
	return Capabilities{Cancel: false}
}

// opencodeReadOnlyConfig deniega edición/escritura/bash/webfetch vía
// OPENCODE_CONFIG_CONTENT: opencode no tiene flag de solo-lectura y por
// default edita igual, así que esta es la única forma verificada de
// imponerlo.
const opencodeReadOnlyConfig = `{"permission":{"edit":"deny","write":"deny","bash":"deny","webfetch":"deny"}}`

// Build arma argv fijo para `opencode run`. read_only=false agrega --auto
// para auto-aprobar permisos (necesario para ejecutar comandos/tests sin
// colgarse esperando aprobación); read_only=true impone el modo lectura vía
// ProcessSpec.Env con OPENCODE_CONFIG_CONTENT.
func (OpenCodeAdapter) Build(req StartRequest, worktreeDir string) (ProcessSpec, error) {
	path, err := lookPath("opencode")
	if err != nil {
		return ProcessSpec{}, ErrAdapterUnavailable
	}

	args := []string{"run", "--format", "json", "--pure", "--dir", worktreeDir}
	if req.Model != "" {
		args = append(args, "-m", req.Model)
	}
	if req.Effort != "" {
		args = append(args, "--variant", req.Effort)
	}

	var env []string
	if req.ReadOnly {
		env = []string{"OPENCODE_CONFIG_CONTENT=" + opencodeReadOnlyConfig}
	} else {
		args = append(args, "--auto")
	}

	return ProcessSpec{
		Path:  path,
		Args:  args,
		Dir:   worktreeDir,
		Stdin: []byte(req.Prompt),
		Env:   env,
	}, nil
}

// opencodeEvent es una línea JSON de salida de `opencode run --format json`;
// solo los eventos type=="text" importan para extraer el resultado.
type opencodeEvent struct {
	Type string        `json:"type"`
	Part *opencodePart `json:"part"`
}

type opencodePart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Result concatena, en orden, el part.text de todos los eventos type=="text".
// Líneas que no parsean como JSON se ignoran. Sin ningún evento de texto,
// devuelve la salida cruda tal cual.
func (OpenCodeAdapter) Result(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	found := false

	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev opencodeEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.Type == "text" && ev.Part != nil {
			buf.WriteString(ev.Part.Text)
			found = true
		}
	}

	if found {
		return buf.Bytes(), nil
	}
	return raw, nil
}
