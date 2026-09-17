package runtime

import (
	"path/filepath"
	"time"

	"github.com/ChampiP/Cliostra/internal/adapters"
)

// task es el trabajo encolado en memoria: el registro persistido más el
// prompt (que nunca se persiste tal cual en el store).
type task struct {
	job    *Job
	prompt string
}

func (r *Runtime) workerLoop() {
	defer r.wg.Done()
	for {
		r.queueMu.Lock()
		for len(r.queue) == 0 && !r.closed {
			r.queueCond.Wait()
		}
		if len(r.queue) == 0 && r.closed {
			r.queueMu.Unlock()
			return
		}
		t := r.queue[0]
		r.queue = r.queue[1:]
		r.queueMu.Unlock()

		r.execute(t)
	}
}

func (r *Runtime) fail(j *Job, reason string, truncated bool) {
	j.State = StateFailed
	j.Reason = reason
	j.Truncated = truncated
	j.UpdatedAt = time.Now()
	_ = r.store.Save(j)
}

func (r *Runtime) execute(t *task) {
	j := t.job
	if err := Transition(j.State, StatePreparing); err != nil {
		r.fail(j, "invalid_transition", false)
		return
	}
	j.State = StatePreparing
	j.UpdatedAt = time.Now()
	if err := r.store.Save(j); err != nil {
		return
	}

	worktreeDir := filepath.Join(r.worktreeRoot, j.ID)
	repo, oid, err := prepareWorktree(j.Repo, worktreeDir, j.ReadOnly)
	if err != nil {
		r.fail(j, "worktree_error: "+err.Error(), false)
		return
	}
	defer cleanupWorktree(repo, worktreeDir)
	_ = oid

	adapter := r.adapters[j.Adapter]
	spec, err := adapter.Build(adapters.StartRequest{Repo: j.Repo, Prompt: t.prompt, ReadOnly: j.ReadOnly, Model: j.Model, Effort: j.Effort}, worktreeDir)
	if err != nil {
		r.fail(j, "adapter_error: "+err.Error(), false)
		return
	}

	if err := Transition(j.State, StateRunning); err != nil {
		r.fail(j, "invalid_transition", false)
		return
	}
	j.State = StateRunning
	j.UpdatedAt = time.Now()
	if err := r.store.Save(j); err != nil {
		return
	}

	ph := &procHandle{cancel: make(chan struct{}), adapter: adapter}
	r.procsMu.Lock()
	r.procs[j.ID] = ph
	r.procsMu.Unlock()
	defer func() {
		r.procsMu.Lock()
		delete(r.procs, j.ID)
		r.procsMu.Unlock()
	}()

	out, truncated, canceled, runErr := runProcess(spec, ph)
	if canceled {
		j.State = StateCanceled
		j.Reason = "canceled_by_client"
		j.Truncated = truncated
		j.UpdatedAt = time.Now()
		_ = r.store.Save(j)
		return
	}
	if truncated {
		r.fail(j, "stream_limit_exceeded", true)
		return
	}
	if runErr != nil {
		r.fail(j, "process_error: "+runErr.Error(), false)
		return
	}

	result, err := adapter.Result(out)
	if err != nil {
		r.fail(j, "result_error: "+err.Error(), false)
		return
	}
	if len(result) > MaxResultSize {
		r.fail(j, "result_limit_exceeded", true)
		return
	}

	j.State = StateSucceeded
	j.Result = string(result)
	if !j.ReadOnly {
		// El worktree sigue vivo hasta que retorne execute() (cleanup
		// diferido), así que el diff todavía refleja lo que el adaptador
		// escribió antes de que se elimine.
		if diff, err := gitDiff(worktreeDir); err == nil {
			j.Diff = diff
		}
	}
	j.UpdatedAt = time.Now()
	_ = r.store.Save(j)
}
