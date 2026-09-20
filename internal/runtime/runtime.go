package runtime

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/ChampiP/Cliostra/internal/adapters"
)

// Límites de tamaño del MVP: exceder cualquiera termina el trabajo en
// failed con Truncated=true, nunca en canceled.
const (
	MaxPromptSize = 64 * 1024
	MaxStreamSize = 8 * 1024 * 1024
	MaxResultSize = 1 * 1024 * 1024
)

// Errores devueltos por Start antes de crear proceso o worktree alguno.
var (
	ErrUnknownAdapter = errors.New("adaptador desconocido")
	ErrPromptTooLarge = errors.New("prompt excede el límite de 64KiB")
	ErrJobNotFound    = errors.New("trabajo no encontrado")
)

// StartRequest es la solicitud de arranque de un trabajo.
type StartRequest struct {
	Adapter  string
	Repo     string
	Prompt   string
	ReadOnly bool
	Model    string
	Effort   string
}

// Config configura una instancia de Runtime.
type Config struct {
	StateDir     string
	WorktreeRoot string
	Workers      int
	Adapters     map[string]adapters.Adapter
}

// Runtime orquesta el pool de workers, la persistencia y la ejecución de
// trabajos sobre el repositorio compartido o worktrees administrados.
type Runtime struct {
	store        *Store
	worktreeRoot string
	adapters     map[string]adapters.Adapter
	workers      int

	queueMu   sync.Mutex
	queueCond *sync.Cond
	queue     []*task
	closed    bool

	procsMu sync.Mutex
	procs   map[string]*procHandle

	wg sync.WaitGroup
}

// New crea el Runtime, ejecuta la recuperación de trabajos no terminales y
// arranca el pool acotado de workers.
func New(cfg Config) (*Runtime, error) {
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	store, err := NewStore(cfg.StateDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.WorktreeRoot, 0o700); err != nil {
		return nil, err
	}
	r := &Runtime{
		store:        store,
		worktreeRoot: cfg.WorktreeRoot,
		adapters:     cfg.Adapters,
		workers:      cfg.Workers,
		procs:        map[string]*procHandle{},
	}
	r.queueCond = sync.NewCond(&r.queueMu)

	if err := r.recover(); err != nil {
		return nil, err
	}
	for i := 0; i < cfg.Workers; i++ {
		r.wg.Add(1)
		go r.workerLoop()
	}
	return r, nil
}

// recover marca como failed/daemon_restarted cualquier trabajo persistido en
// estado no terminal: el demonio no reanuda ejecuciones interrumpidas.
func (r *Runtime) recover() error {
	jobs, err := r.store.LoadAll()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if j.State.IsTerminal() {
			continue
		}
		j.State = StateFailed
		j.Reason = "daemon_restarted"
		j.UpdatedAt = time.Now()
		if err := r.store.Save(j); err != nil {
			return err
		}
	}
	return nil
}

// Close detiene el pool de workers de forma ordenada, esperando a que
// terminen los trabajos en curso.
func (r *Runtime) Close() {
	r.queueMu.Lock()
	r.closed = true
	r.queueCond.Broadcast()
	r.queueMu.Unlock()
	r.wg.Wait()
}

func newJobID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Start valida la solicitud, persiste el estado inicial `queued` y encola el
// trabajo sin esperar su ejecución. La cola en memoria no tiene límite fijo:
// el backlog nunca bloquea a Start, solo se acumula como `queued`.
func (r *Runtime) Start(req StartRequest) (string, error) {
	if len(req.Prompt) > MaxPromptSize {
		return "", ErrPromptTooLarge
	}
	if _, ok := r.adapters[req.Adapter]; !ok {
		return "", ErrUnknownAdapter
	}

	now := time.Now()
	j := &Job{
		ID:        newJobID(),
		Adapter:   req.Adapter,
		Repo:      req.Repo,
		ReadOnly:  req.ReadOnly,
		Model:     req.Model,
		Effort:    req.Effort,
		State:     StateQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := r.store.Save(j); err != nil {
		return "", err
	}

	r.queueMu.Lock()
	r.queue = append(r.queue, &task{job: j, prompt: req.Prompt})
	r.queueCond.Signal()
	r.queueMu.Unlock()

	return j.ID, nil
}

// Status devuelve el registro durable actual del trabajo.
func (r *Runtime) Status(id string) (*Job, error) {
	j, err := r.store.Load(id)
	if err != nil {
		return nil, ErrJobNotFound
	}
	return j, nil
}

// Cancel solicita la cancelación de un trabajo. Solo informa cancelación
// real cuando el adaptador la soporta y el proceso efectivamente termina;
// para adaptadores sin esa capacidad devuelve supported=false sin tocar el
// estado persistido.
func (r *Runtime) Cancel(id string) (job *Job, supported bool, err error) {
	j, err := r.store.Load(id)
	if err != nil {
		return nil, false, ErrJobNotFound
	}

	r.procsMu.Lock()
	ph, running := r.procs[id]
	r.procsMu.Unlock()

	if !running {
		// No hay proceso activo: si ya es terminal, se informa tal cual;
		// si sigue queued, no hay capacidad que consultar todavía.
		return j, j.State.IsTerminal(), nil
	}
	if !ph.adapter.Capabilities().Cancel {
		return j, false, nil
	}

	if err := Transition(j.State, StateCanceling); err != nil {
		// ya está en canceling o es terminal; se devuelve el estado actual.
		return j, true, nil
	}
	j.State = StateCanceling
	j.UpdatedAt = time.Now()
	if err := r.store.Save(j); err != nil {
		return nil, false, err
	}
	ph.signalCancel()

	// Bloquea hasta que el worker detecte la cancelación y el proceso salga;
	// se refleja al releer el estado persistido.
	for {
		latest, err := r.store.Load(id)
		if err != nil {
			return nil, false, err
		}
		if latest.State.IsTerminal() {
			return latest, true, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
}
