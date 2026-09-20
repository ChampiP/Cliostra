package runtime

import "testing"

func TestTransitionValidPath(t *testing.T) {
	steps := []State{StateQueued, StatePreparing, StateRunning, StateSucceeded}
	for i := 1; i < len(steps); i++ {
		if err := Transition(steps[i-1], steps[i]); err != nil {
			t.Fatalf("transición %s->%s debería ser válida: %v", steps[i-1], steps[i], err)
		}
	}
}

func TestTransitionCancelPath(t *testing.T) {
	if err := Transition(StateQueued, StateCanceled); err != nil {
		t.Fatalf("queued->canceled debería ser válida: %v", err)
	}
	if err := Transition(StateRunning, StateCanceling); err != nil {
		t.Fatalf("running->canceling debería ser válida: %v", err)
	}
	if err := Transition(StateCanceling, StateCanceled); err != nil {
		t.Fatalf("canceling->canceled debería ser válida: %v", err)
	}
}

func TestTransitionRejectsInvalidJump(t *testing.T) {
	if err := Transition(StateQueued, StateSucceeded); err == nil {
		t.Fatal("queued->succeeded debería rechazarse")
	}
	if err := Transition(StateSucceeded, StateRunning); err == nil {
		t.Fatal("un estado terminal no debería admitir transición")
	}
}

func TestTransitionIsIdempotent(t *testing.T) {
	if err := Transition(StateRunning, StateRunning); err != nil {
		t.Fatalf("reaplicar el mismo estado no debería fallar: %v", err)
	}
}

func TestIsTerminal(t *testing.T) {
	terminal := []State{StateSucceeded, StateFailed, StateCanceled}
	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Fatalf("%s debería ser terminal", s)
		}
	}
	nonTerminal := []State{StateQueued, StatePreparing, StateRunning, StateCanceling}
	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Fatalf("%s no debería ser terminal", s)
		}
	}
}
