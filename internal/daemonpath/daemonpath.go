// Paquete daemonpath centraliza las rutas XDG que usan tanto cliostrad como
// cliostra, para no duplicar la resolución de directorios en ambos binarios.
package daemonpath

import (
	"os"
	"path/filepath"
)

// RuntimeDir devuelve XDG_RUNTIME_DIR, o un fallback bajo el temporal del
// sistema si no está definido.
func RuntimeDir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return d
	}
	return filepath.Join(os.TempDir(), "cliostra-run")
}

// StateDir devuelve XDG_STATE_HOME/cliostra.
func StateDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "cliostra")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "cliostra")
}

// CacheDir devuelve XDG_CACHE_HOME/cliostra.
func CacheDir() string {
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, "cliostra")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "cliostra")
}

// SocketPath devuelve la ruta del socket Unix privado del demonio.
func SocketPath() string {
	return filepath.Join(RuntimeDir(), "cliostra.sock")
}
