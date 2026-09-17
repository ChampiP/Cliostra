package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/ChampiP/Cliostra/internal/adapters"
)

// procHandle referencia el proceso en curso de un trabajo para permitir su
// cancelación desde Runtime.Cancel.
type procHandle struct {
	cmd     *exec.Cmd
	cancel  chan struct{}
	once    sync.Once
	adapter adapters.Adapter
}

// signalCancel cierra el canal de cancelación una sola vez. No alcanza con
// apoyarse en Transition: es idempotente (from==to devuelve nil), así que dos
// Cancel concurrentes llegaban ambos al close y tumbaban el demonio.
func (p *procHandle) signalCancel() {
	p.once.Do(func() { close(p.cancel) })
}

// limitedBuffer acota la cantidad de bytes acumulados de stdout+stderr; al
// excederse, señala overflow para que el llamador termine el proceso.
type limitedBuffer struct {
	mu         sync.Mutex
	buf        []byte
	limit      int
	overflow   bool
	onOverflow func()
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.overflow {
		return len(p), nil
	}
	if len(b.buf)+len(p) > b.limit {
		b.buf = append(b.buf, p[:b.limit-len(b.buf)]...)
		b.overflow = true
		if b.onOverflow != nil {
			go b.onOverflow()
		}
		return len(p), nil
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf...)
}

func (b *limitedBuffer) Overflowed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.overflow
}

// runProcess ejecuta spec sin shell, alimenta el prompt por stdin, acota la
// salida combinada a MaxStreamSize y atiende cancelación real (TERM→KILL)
// solo tras la salida del proceso.
func runProcess(spec adapters.ProcessSpec, ph *procHandle) (out []byte, truncated bool, canceled bool, err error) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	cmd := exec.CommandContext(ctx, spec.Path, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = append(filteredEnv(), spec.Env...)
	// bytes.Reader y no *os.File: con un io.Reader, os/exec crea y cierra el
	// pipe él mismo. Con un *os.File el extremo de lectura queda a cargo del
	// llamador, y nadie lo cerraba (un fd filtrado por trabajo).
	cmd.Stdin = bytes.NewReader(spec.Stdin)

	lb := &limitedBuffer{limit: MaxStreamSize}
	lb.onOverflow = func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	cmd.Stdout = lb
	cmd.Stderr = lb

	if err := cmd.Start(); err != nil {
		return nil, false, false, err
	}
	ph.cmd = cmd

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	select {
	case <-ph.cancel:
		if cmd.Process != nil {
			_ = cmd.Process.Signal(processTerminateSignal())
		}
		select {
		case <-waitCh:
		case <-time.After(5 * time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-waitCh
		}
		return lb.Bytes(), lb.Overflowed(), true, nil
	case werr := <-waitCh:
		if lb.Overflowed() {
			return lb.Bytes(), true, false, nil
		}
		return lb.Bytes(), false, false, werr
	}
}

// processTerminateSignal es SIGTERM: primer paso de la cancelación real
// (TERM→KILL), aplicado solo tras la salida efectiva del proceso.
func processTerminateSignal() os.Signal {
	return syscall.SIGTERM
}

// filteredEnv hereda solo variables de entorno seguras (PATH, HOME, usuario,
// temporales y locale); nunca copia el entorno completo del demonio.
func filteredEnv() []string {
	allow := []string{"PATH", "HOME", "USER", "LOGNAME", "TMPDIR", "LANG", "LC_ALL", "LC_CTYPE"}
	var env []string
	for _, k := range allow {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return env
}
