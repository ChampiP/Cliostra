// Paquete adapters define los adaptadores experimentales que traducen una
// solicitud de trabajo en un proceso concreto (Claude Code o `agy`), sin
// invocar shell y sin filtrar variables de entorno o secretos.
package adapters

import (
	"errors"
	"os/exec"
)

// Capabilities declara qué soporta un adaptador. Una capacidad no declarada
// nunca debe asumirse disponible.
type Capabilities struct {
	Cancel bool
}

// ProcessSpec describe el proceso a lanzar: argv fijo, sin shell, prompt por
// stdin.
type ProcessSpec struct {
	Path  string
	Args  []string
	Dir   string
	Stdin []byte
	// Env agrega variables de entorno específicas del adaptador (formato
	// "CLAVE=valor"), sumadas al entorno base restringido de runtime. nil no
	// cambia el comportamiento actual.
	Env []string
}

// StartRequest es la solicitud de trabajo tal como llega desde internal/api,
// duplicada aquí para no acoplar adapters a runtime.
type StartRequest struct {
	Repo     string
	Prompt   string
	ReadOnly bool
	Model    string
	Effort   string
}

// Adapter traduce una solicitud en un proceso ejecutable y sabe interpretar
// su resultado crudo.
type Adapter interface {
	Name() string
	Capabilities() Capabilities
	Build(req StartRequest, worktreeDir string) (ProcessSpec, error)
	Result(raw []byte) ([]byte, error)
}

// ErrAdapterUnavailable se devuelve cuando el binario del adaptador no está
// instalado.
var ErrAdapterUnavailable = errors.New("adaptador no disponible")

// lookPath es una variable para poder sustituirla en pruebas.
var lookPath = exec.LookPath

// Registry expone los adaptadores disponibles por nombre.
func Registry() map[string]Adapter {
	return map[string]Adapter{
		"claude-code": ClaudeCodeAdapter{},
		"agy":         AgyAdapter{},
		"codex":       CodexAdapter{},
		"opencode":    OpenCodeAdapter{},
	}
}
