//go:build !windows

package runtime

import (
	"os"
	"os/exec"
	"syscall"
)

// setProcessGroup configura el grupo de procesos en sistemas POSIX/Unix.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// processTerminateSignal es SIGTERM: primer paso de la cancelación real
// (TERM→KILL), aplicado solo tras la salida efectiva del proceso.
func processTerminateSignal() os.Signal {
	return syscall.SIGTERM
}

// terminateProcessGroup envía la señal de terminación al grupo de procesos.
func terminateProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return
	}
	sig, ok := processTerminateSignal().(syscall.Signal)
	if !ok {
		sig = syscall.SIGTERM
	}
	if err := syscall.Kill(-cmd.Process.Pid, sig); err != nil {
		_ = cmd.Process.Signal(processTerminateSignal())
	}
}

// killProcessGroup fuerza la muerte de todo el grupo de procesos para no
// dejar hijos huérfanos con pipes abiertos.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}
