// cliostrad es el demonio por usuario: escucha en un socket Unix privado
// (0600) bajo XDG_RUNTIME_DIR, corre la recuperación de trabajos al arrancar
// y atiende conexiones RPC de la CLI y del servidor MCP.
package main

import (
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/ChampiP/Cliostra/internal/adapters"
	"github.com/ChampiP/Cliostra/internal/api"
	"github.com/ChampiP/Cliostra/internal/daemonpath"
	"github.com/ChampiP/Cliostra/internal/mcpsync"
	"github.com/ChampiP/Cliostra/internal/runtime"
)

// syncAgyMcpTimeout acota cuánto puede tardar la sincronización de MCP de
// agy al arrancar: nunca debe colgar el arranque del demonio.
const syncAgyMcpTimeout = 30 * time.Second

func main() {
	go syncAgyMcpWithTimeout()

	sockPath := daemonpath.SocketPath()
	if err := os.MkdirAll(filepath.Dir(sockPath), 0o700); err != nil {
		log.Fatalf("no se pudo crear directorio de runtime: %v", err)
	}
	_ = os.Remove(sockPath)

	rt, err := runtime.New(runtime.Config{
		StateDir:     filepath.Join(daemonpath.StateDir(), "jobs"),
		WorktreeRoot: filepath.Join(daemonpath.CacheDir(), "worktrees"),
		Workers:      4,
		Adapters:     adapters.Registry(),
	})
	if err != nil {
		log.Fatalf("no se pudo inicializar runtime: %v", err)
	}
	defer rt.Close()

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		log.Fatalf("no se pudo escuchar en %s: %v", sockPath, err)
	}
	defer ln.Close()
	if err := os.Chmod(sockPath, 0o600); err != nil {
		log.Fatalf("no se pudo restringir permisos del socket: %v", err)
	}

	handlers := runtime.Handlers(rt)
	log.Printf("cliostrad escuchando en %s", sockPath)
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go func() {
			defer conn.Close()
			if err := api.Serve(conn, handlers); err != nil {
				log.Printf("conexión terminada con error: %v", err)
			}
		}()
	}
}

// syncAgyMcpWithTimeout corre la sincronización de servidores MCP de agy en
// background, sin bloquear el arranque del demonio. Un fallo o una demora se
// loguean y nunca impiden que cliostrad levante.
func syncAgyMcpWithTimeout() {
	done := make(chan error, 1)
	go func() { done <- mcpsync.SyncAgy(log.Default()) }()

	select {
	case err := <-done:
		if err != nil {
			log.Printf("mcpsync: fallo al sincronizar MCP de agy: %v", err)
		}
	case <-time.After(syncAgyMcpTimeout):
		log.Printf("mcpsync: sincronización de MCP de agy superó el timeout de %s, se sigue en background", syncAgyMcpTimeout)
	}
}
