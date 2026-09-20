// Paquete mcpsync copia entradas de MCP faltantes del registro de escritorio
// de Antigravity (`agy`) hacia su registro headless, que es el que usa el
// binario `agy` en modo --print (ver internal/adapters/agy.go). Es
// exclusivamente aditivo: nunca borra ni sobreescribe una entrada existente
// en el headless, y nunca toca el registro de escritorio.
package mcpsync

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

// mcpConfig es el esquema compartido por ambos registros de agy.
type mcpConfig struct {
	McpServers map[string]mcpServerEntry `json:"mcpServers"`
}

type mcpServerEntry struct {
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	ServerURL string            `json:"serverUrl,omitempty"`
}

// lookPath es una variable para poder sustituirla en pruebas (mismo patrón
// que internal/adapters/adapters.go).
var lookPath = exec.LookPath

// execCommand es una variable para poder sustituir la ejecución de `agy mcp
// add` en pruebas, sin invocar el binario real.
var execCommand = exec.Command

// readMcpConfig lee un archivo de configuración MCP de agy. Un archivo
// ausente no es un error: devuelve un mapa vacío.
func readMcpConfig(path string) (map[string]mcpServerEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]mcpServerEntry{}, nil
		}
		return nil, err
	}
	var cfg mcpConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.McpServers == nil {
		return map[string]mcpServerEntry{}, nil
	}
	return cfg.McpServers, nil
}

// missingEntries lee los dos registros (desktopPath: fuente de verdad,
// headlessPath: el que usa el binario `agy`) y devuelve las entradas
// presentes en el de escritorio pero ausentes en el headless, con nombre.
func missingEntries(desktopPath, headlessPath string) (map[string]mcpServerEntry, error) {
	desktop, err := readMcpConfig(desktopPath)
	if err != nil {
		return nil, err
	}
	if len(desktop) == 0 {
		return nil, nil
	}
	headless, err := readMcpConfig(headlessPath)
	if err != nil {
		return nil, err
	}

	missing := make(map[string]mcpServerEntry)
	for name, entry := range desktop {
		if _, ok := headless[name]; !ok {
			missing[name] = entry
		}
	}
	return missing, nil
}

// addArgs traduce una entrada de MCP en el argv de
// `agy mcp add [--env K=V...] <nombre> <comandoOUrl> [args...]`.
func addArgs(name string, entry mcpServerEntry) []string {
	var flags []string
	if len(entry.Env) > 0 {
		keys := make([]string, 0, len(entry.Env))
		for k := range entry.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			flags = append(flags, "--env", fmt.Sprintf("%s=%s", k, entry.Env[k]))
		}
	}
	base := append([]string{"mcp", "add"}, flags...)
	if entry.ServerURL != "" {
		return append(base, name, entry.ServerURL)
	}
	base = append(base, name, entry.Command)
	base = append(base, entry.Args...)
	return base
}

// SyncAgy sincroniza el registro headless de agy con el de escritorio: copia
// cada servidor MCP que el escritorio tiene y el headless no, invocando `agy
// mcp add` como subproceso (nunca reescribe el JSON a mano). Si `agy` no está
// instalado, o si el registro de escritorio no existe, no hace nada. Un
// fallo al agregar una entrada puntual se loguea y no aborta el resto.
func SyncAgy(logger *log.Logger) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	desktopPath := filepath.Join(home, ".gemini", "antigravity-cli", "mcp_config.json")
	headlessPath := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	return syncAgyPaths(logger, desktopPath, headlessPath)
}

// syncAgyPaths es la implementación testeable de SyncAgy: toma rutas
// explícitas en vez de resolver os.UserHomeDir().
func syncAgyPaths(logger *log.Logger, desktopPath, headlessPath string) error {
	if _, err := lookPath("agy"); err != nil {
		return nil
	}

	missing, err := missingEntries(desktopPath, headlessPath)
	if err != nil {
		return err
	}

	for name, entry := range missing {
		cmd := execCommand("agy", addArgs(name, entry)...)
		if err := cmd.Run(); err != nil {
			logger.Printf("mcpsync: no se pudo copiar el servidor MCP %q al registro headless de agy: %v", name, err)
			continue
		}
		logger.Printf("mcpsync: servidor MCP %q copiado al registro headless de agy", name)
	}
	return nil
}
