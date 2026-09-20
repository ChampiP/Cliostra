//go:build windows

package runtime

import (
	"os"
	"os/exec"
)

// setProcessGroup en Windows no requiere Setpgid; los procesos se administran
// directamente o mediante Job Objects según la configuración de la consola.
func setProcessGroup(cmd *exec.Cmd) {
}

// processTerminateSignal devuelve os.Kill en Windows, dado que syscall.SIGTERM
// no existe como señal nativa y os.Process.Signal solo soporta os.Kill en esta plataforma.
func processTerminateSignal() os.Signal {
	return os.Kill
}

// terminateProcessGroup termina el proceso en Windows.
func terminateProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}

// killProcessGroup fuerza la terminación del proceso en Windows.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
