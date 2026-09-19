package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ChampiP/Cliostra/internal/adapters"
)

// fakeAdapter es un adaptador de prueba: no depende de Claude Code ni agy.
type fakeAdapter struct {
	cancel   bool
	script   string // script de shell embebido para simular el proceso
	buildErr error
	env      []string // variables extra que el adaptador declara en ProcessSpec.Env
}

func (f fakeAdapter) Name() string { return "fake" }
func (f fakeAdapter) Capabilities() adapters.Capabilities {
	return adapters.Capabilities{Cancel: f.cancel}
}
func (f fakeAdapter) Build(req adapters.StartRequest, worktreeDir string) (adapters.ProcessSpec, error) {
	if f.buildErr != nil {
		return adapters.ProcessSpec{}, f.buildErr
	}
	script := f.script
	if script == "" {
		script = `cat >/dev/null; echo ok`
	}
	return adapters.ProcessSpec{
		Path:  "/bin/sh",
		Args:  []string{"-c", script},
		Dir:   worktreeDir,
		Stdin: []byte(req.Prompt),
		Env:   f.env,
	}, nil
}
func (f fakeAdapter) Result(raw []byte) ([]byte, error) {
	return []byte(strings.TrimSpace(string(raw))), nil
}

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hola"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "f.txt")
	run("commit", "-m", "init")
	return dir
}

func newTestRuntime(t *testing.T, adapterMap map[string]adapters.Adapter, workers int) *Runtime {
	t.Helper()
	stateDir := t.TempDir()
	worktreeRoot := t.TempDir()
	rt, err := New(Config{StateDir: stateDir, WorktreeRoot: worktreeRoot, Workers: workers, Adapters: adapterMap})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(rt.Close)
	return rt
}

func waitTerminal(t *testing.T, rt *Runtime, id string) *Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		j, err := rt.Status(id)
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if j.State.IsTerminal() {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout esperando estado terminal")
	return nil
}

func TestStartSucceedsEndToEnd(t *testing.T) {
	repo := initTestRepo(t)
	rt := newTestRuntime(t, map[string]adapters.Adapter{"fake": fakeAdapter{}}, 2)

	id, err := rt.Start(StartRequest{Adapter: "fake", Repo: repo, Prompt: "hola", ReadOnly: true})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if id == "" {
		t.Fatal("Start debe devolver un ID")
	}

	j := waitTerminal(t, rt, id)
	if j.State != StateSucceeded {
		t.Fatalf("estado inesperado: %+v", j)
	}
	if j.Result != "ok" {
		t.Fatalf("resultado inesperado: %q", j.Result)
	}
}

func TestWriteModeSkipsStripWriteAndCapturesDiff(t *testing.T) {
	repo := initTestRepo(t)
	editScript := `echo cambiado > f.txt; echo listo`
	rt := newTestRuntime(t, map[string]adapters.Adapter{"fake": fakeAdapter{script: editScript}}, 1)

	id, err := rt.Start(StartRequest{Adapter: "fake", Repo: repo, Prompt: "hola", ReadOnly: false})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	j := waitTerminal(t, rt, id)
	if j.State != StateSucceeded {
		t.Fatalf("estado inesperado: %+v", j)
	}
	if !strings.Contains(j.Diff, "cambiado") {
		t.Fatalf("diff debe reflejar la edición real: %q", j.Diff)
	}
	// El repo real del usuario nunca se toca: el worktree es descartable.
	original, err := os.ReadFile(filepath.Join(repo, "f.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(original)) != "hola" {
		t.Fatalf("el repo real no debe modificarse, got %q", original)
	}
}

// El diff es lo que el usuario revisa antes de aplicar el cambio a su rama,
// así que no debe venir mezclado con andamiaje que generan los propios CLIs
// de agente al arrancar en cualquier directorio (verificado en vivo: tanto
// Claude Code como Codex crean .atl/ por la config global del usuario).
func TestDiffExcludesAgentScaffolding(t *testing.T) {
	repo := initTestRepo(t)
	script := `mkdir -p .atl && echo '{"fingerprint":"x"}' > .atl/cache.json; echo cambiado > f.txt; echo listo`
	rt := newTestRuntime(t, map[string]adapters.Adapter{"fake": fakeAdapter{script: script}}, 1)

	id, err := rt.Start(StartRequest{Adapter: "fake", Repo: repo, Prompt: "x", ReadOnly: false})
	if err != nil {
		t.Fatal(err)
	}
	j := waitTerminal(t, rt, id)
	if j.State != StateSucceeded {
		t.Fatalf("estado inesperado: %+v", j)
	}
	if !strings.Contains(j.Diff, "cambiado") {
		t.Fatalf("el diff debe conservar el cambio real: %q", j.Diff)
	}
	if strings.Contains(j.Diff, ".atl") {
		t.Fatalf("el diff no debe incluir andamiaje de agentes: %q", j.Diff)
	}
}

// codex y agy trabajan directo sobre el repo real, no en un worktree
// descartable (ver directRepoAdapters): sus propios CLIs no respetan el
// aislamiento (agy) o el worktree armado desde HEAD les esconde archivos sin
// commitear (codex). Este test fija ese contrato: el proceso debe correr con
// Dir = raíz del repo, y el cambio debe quedar en el repo real.
func TestDirectRepoAdaptersSkipWorktree(t *testing.T) {
	repo := initTestRepo(t)
	editScript := `echo directo > f.txt; echo listo`
	rt := newTestRuntime(t, map[string]adapters.Adapter{"codex": fakeAdapter{script: editScript}}, 1)

	id, err := rt.Start(StartRequest{Adapter: "codex", Repo: repo, Prompt: "hola", ReadOnly: false})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	j := waitTerminal(t, rt, id)
	if j.State != StateSucceeded {
		t.Fatalf("estado inesperado: %+v", j)
	}
	// Sin worktree aislado, no hay diff que capturar: el cambio ya está en
	// el repo real.
	if j.Diff != "" {
		t.Fatalf("modo directo no debe capturar diff: %q", j.Diff)
	}
	changed, err := os.ReadFile(filepath.Join(repo, "f.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(changed)) != "directo" {
		t.Fatalf("el cambio debe aplicarse al repo real, got %q", changed)
	}
}

func TestReadOnlyModeOmitsDiff(t *testing.T) {
	repo := initTestRepo(t)
	rt := newTestRuntime(t, map[string]adapters.Adapter{"fake": fakeAdapter{}}, 1)

	id, err := rt.Start(StartRequest{Adapter: "fake", Repo: repo, Prompt: "hola", ReadOnly: true})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	j := waitTerminal(t, rt, id)
	if j.Diff != "" {
		t.Fatalf("modo solo lectura no debe capturar diff, got %q", j.Diff)
	}
}

func TestStartRejectsUnknownAdapter(t *testing.T) {
	rt := newTestRuntime(t, map[string]adapters.Adapter{}, 1)
	_, err := rt.Start(StartRequest{Adapter: "nope", ReadOnly: true})
	if err != ErrUnknownAdapter {
		t.Fatalf("esperado ErrUnknownAdapter, got %v", err)
	}
}

func TestStartRejectsOversizedPrompt(t *testing.T) {
	rt := newTestRuntime(t, map[string]adapters.Adapter{"fake": fakeAdapter{}}, 1)
	_, err := rt.Start(StartRequest{Adapter: "fake", ReadOnly: true, Prompt: strings.Repeat("a", MaxPromptSize+1)})
	if err != ErrPromptTooLarge {
		t.Fatalf("esperado ErrPromptTooLarge, got %v", err)
	}
}

func TestStatusUnknownIDReturnsNotFound(t *testing.T) {
	rt := newTestRuntime(t, map[string]adapters.Adapter{}, 1)
	_, err := rt.Status("no-existe")
	if err != ErrJobNotFound {
		t.Fatalf("esperado ErrJobNotFound, got %v", err)
	}
}

func TestStartNeverBlocksUnderBacklog(t *testing.T) {
	repo := initTestRepo(t)
	// Un solo worker y trabajos lentos: el backlog debe quedar `queued`
	// sin bloquear Start.
	rt := newTestRuntime(t, map[string]adapters.Adapter{"fake": fakeAdapter{script: "sleep 0.3; echo ok"}}, 1)

	start := time.Now()
	var ids []string
	for i := 0; i < 5; i++ {
		id, err := rt.Start(StartRequest{Adapter: "fake", Repo: repo, Prompt: "x", ReadOnly: true})
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		ids = append(ids, id)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("Start debería devolver de inmediato; tardó %v", elapsed)
	}

	// El primero puede ya estar preparing/running; el resto debe seguir queued.
	queuedCount := 0
	for _, id := range ids {
		j, err := rt.Status(id)
		if err != nil {
			t.Fatal(err)
		}
		if j.State == StateQueued {
			queuedCount++
		}
	}
	if queuedCount == 0 {
		t.Fatal("se esperaba backlog en estado queued con un solo worker")
	}

	for _, id := range ids {
		waitTerminal(t, rt, id)
	}
}

// Regresión: ProcessSpec.Env (usado por opencode.go para imponer permisos
// de solo lectura) debe llegar de verdad al proceso hijo, sumado al entorno
// base restringido.
func TestAdapterDeclaredEnvReachesChildProcess(t *testing.T) {
	repo := initTestRepo(t)
	rt := newTestRuntime(t, map[string]adapters.Adapter{
		"fake": fakeAdapter{script: `echo "$CLIOSTRA_TEST_VAR"`, env: []string{"CLIOSTRA_TEST_VAR=propagada"}},
	}, 1)

	id, err := rt.Start(StartRequest{Adapter: "fake", Repo: repo, Prompt: "x", ReadOnly: true})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	j := waitTerminal(t, rt, id)
	if j.State != StateSucceeded {
		t.Fatalf("estado inesperado: %+v", j)
	}
	if j.Result != "propagada" {
		t.Fatalf("la variable de entorno declarada por el adaptador no llegó al proceso hijo, got %q", j.Result)
	}
}

func TestModelAndEffortSurvivePersistence(t *testing.T) {
	repo := initTestRepo(t)
	rt := newTestRuntime(t, map[string]adapters.Adapter{"fake": fakeAdapter{}}, 1)

	id, err := rt.Start(StartRequest{Adapter: "fake", Repo: repo, Prompt: "x", ReadOnly: true, Model: "sonnet", Effort: "high"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	j, err := rt.Status(id)
	if err != nil {
		t.Fatal(err)
	}
	if j.Model != "sonnet" || j.Effort != "high" {
		t.Fatalf("model/effort deben persistirse y releerse, got %+v", j)
	}
	waitTerminal(t, rt, id)
}

func TestRestartRecoveryMarksNonTerminalFailed(t *testing.T) {
	stateDir := t.TempDir()
	worktreeRoot := t.TempDir()

	store, err := NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	stuck := &Job{ID: "stuck1", Adapter: "fake", State: StateRunning, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := store.Save(stuck); err != nil {
		t.Fatal(err)
	}

	rt, err := New(Config{StateDir: stateDir, WorktreeRoot: worktreeRoot, Workers: 1, Adapters: map[string]adapters.Adapter{}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer rt.Close()

	j, err := rt.Status("stuck1")
	if err != nil {
		t.Fatal(err)
	}
	if j.State != StateFailed || j.Reason != "daemon_restarted" {
		t.Fatalf("esperado failed/daemon_restarted, got %+v", j)
	}
}

func TestQueryAfterRestartReturnsSameTerminalState(t *testing.T) {
	stateDir := t.TempDir()
	worktreeRoot := t.TempDir()
	store, err := NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	done := &Job{ID: "done1", Adapter: "fake", State: StateSucceeded, Result: "listo", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := store.Save(done); err != nil {
		t.Fatal(err)
	}

	rt, err := New(Config{StateDir: stateDir, WorktreeRoot: worktreeRoot, Workers: 1, Adapters: map[string]adapters.Adapter{}})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	j, err := rt.Status("done1")
	if err != nil {
		t.Fatal(err)
	}
	if j.State != StateSucceeded || j.Result != "listo" {
		t.Fatalf("estado terminal debería sobrevivir al reinicio: %+v", j)
	}
}

func TestWorktreeIsSeparateFromOrigin(t *testing.T) {
	repo := initTestRepo(t)
	rt := newTestRuntime(t, map[string]adapters.Adapter{"fake": fakeAdapter{script: `pwd`}}, 1)

	id, err := rt.Start(StartRequest{Adapter: "fake", Repo: repo, Prompt: "x", ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	j := waitTerminal(t, rt, id)
	if j.State != StateSucceeded {
		t.Fatalf("esperado succeeded, got %+v", j)
	}
	if strings.TrimSpace(j.Result) == repo {
		t.Fatal("el trabajo no debería ejecutarse dentro del repo de origen")
	}
}

func TestStreamLimitExceededFailsTruncated(t *testing.T) {
	repo := initTestRepo(t)
	// Genera más de MaxStreamSize bytes de salida.
	script := "yes x | head -c 9000000"
	rt := newTestRuntime(t, map[string]adapters.Adapter{"fake": fakeAdapter{script: script}}, 1)

	id, err := rt.Start(StartRequest{Adapter: "fake", Repo: repo, Prompt: "x", ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	j := waitTerminal(t, rt, id)
	if j.State != StateFailed || !j.Truncated {
		t.Fatalf("esperado failed truncated=true, got %+v", j)
	}
}

func TestCancelUnsupportedAdapterReturnsUnsupported(t *testing.T) {
	repo := initTestRepo(t)
	rt := newTestRuntime(t, map[string]adapters.Adapter{"fake": fakeAdapter{cancel: false, script: "sleep 1; echo ok"}}, 1)

	id, err := rt.Start(StartRequest{Adapter: "fake", Repo: repo, Prompt: "x", ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	// Espera a que el proceso arranque.
	time.Sleep(150 * time.Millisecond)

	j, supported, err := rt.Cancel(id)
	if err != nil {
		t.Fatal(err)
	}
	if supported {
		t.Fatal("agy-like adapter no debería soportar cancelación")
	}
	if j.State == StateCanceled {
		t.Fatal("no debe afirmarse cancelación inexistente")
	}
}

func TestCancelSupportedAdapterTransitionsToCanceled(t *testing.T) {
	repo := initTestRepo(t)
	rt := newTestRuntime(t, map[string]adapters.Adapter{"fake": fakeAdapter{cancel: true, script: "sleep 1; echo ok"}}, 1)

	id, err := rt.Start(StartRequest{Adapter: "fake", Repo: repo, Prompt: "x", ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)

	j, supported, err := rt.Cancel(id)
	if err != nil {
		t.Fatal(err)
	}
	if !supported {
		t.Fatal("esperado cancelación soportada")
	}
	if j.State != StateCanceled {
		t.Fatalf("esperado canceled, got %+v", j)
	}
}

// Regresión: Transition es idempotente (from==to devuelve nil), así que el
// segundo Cancel concurrente llegaba a close() sobre un canal ya cerrado y
// tumbaba el demonio entero con panic.
func TestConcurrentCancelDoesNotPanic(t *testing.T) {
	repo := initTestRepo(t)
	rt := newTestRuntime(t, map[string]adapters.Adapter{"fake": fakeAdapter{cancel: true, script: "sleep 2; echo ok"}}, 1)

	id, err := rt.Start(StartRequest{Adapter: "fake", Repo: repo, Prompt: "x", ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = rt.Cancel(id)
		}()
	}
	wg.Wait()

	j := waitTerminal(t, rt, id)
	if !j.State.IsTerminal() {
		t.Fatalf("el trabajo debe quedar terminal: %+v", j)
	}
}

// Regresión: cmd.Stdin recibía un *os.File cuyo extremo de lectura nadie
// cerraba, así que cada trabajo filtraba un descriptor de archivo.
func TestRepeatedJobsDoNotLeakFileDescriptors(t *testing.T) {
	repo := initTestRepo(t)
	rt := newTestRuntime(t, map[string]adapters.Adapter{"fake": fakeAdapter{}}, 1)

	openFDs := func() int {
		entries, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Skip("sin /proc/self/fd; no se puede medir fugas de fd")
		}
		return len(entries)
	}

	// Una primera vuelta calienta buffers y caches del runtime.
	for i := 0; i < 3; i++ {
		id, err := rt.Start(StartRequest{Adapter: "fake", Repo: repo, Prompt: "x", ReadOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		waitTerminal(t, rt, id)
	}
	before := openFDs()

	for i := 0; i < 10; i++ {
		id, err := rt.Start(StartRequest{Adapter: "fake", Repo: repo, Prompt: "x", ReadOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		waitTerminal(t, rt, id)
	}
	after := openFDs()

	if after-before > 3 {
		t.Fatalf("fuga de descriptores: %d abiertos antes, %d después de 10 trabajos", before, after)
	}
}
