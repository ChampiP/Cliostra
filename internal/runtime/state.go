// Paquete runtime implementa la máquina de estados, la persistencia durable,
// el pool de workers acotado y la ejecución en worktrees administrados.
package runtime

import "fmt"

// State representa un estado válido de un trabajo.
type State string

const (
	StateQueued    State = "queued"
	StatePreparing State = "preparing"
	StateRunning   State = "running"
	StateSucceeded State = "succeeded"
	StateFailed    State = "failed"
	StateCanceling State = "canceling"
	StateCanceled  State = "canceled"
)

// IsTerminal indica si el estado ya no admite transiciones.
func (s State) IsTerminal() bool {
	switch s {
	case StateSucceeded, StateFailed, StateCanceled:
		return true
	default:
		return false
	}
}

// edges define las transiciones válidas del ciclo de vida:
// queued → preparing → running → succeeded|failed
//
//	running → canceling → canceled
var edges = map[State]map[State]bool{
	StateQueued:    {StatePreparing: true, StateFailed: true},
	StatePreparing: {StateRunning: true, StateFailed: true},
	StateRunning:   {StateSucceeded: true, StateFailed: true, StateCanceling: true},
	StateCanceling: {StateCanceled: true, StateFailed: true},
}

// Transition valida el paso de from a to. Es idempotente: pasar al mismo
// estado actual no es error. Cualquier otro salto no declarado en edges es
// rechazado.
func Transition(from, to State) error {
	if from == to {
		return nil
	}
	if allowed, ok := edges[from]; ok && allowed[to] {
		return nil
	}
	return fmt.Errorf("transición inválida: %s -> %s", from, to)
}
