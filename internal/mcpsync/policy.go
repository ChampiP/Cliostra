package mcpsync

import (
	"strings"
)

// BlockedWorkerMCPServers contiene los nombres de servidores MCP que jamás deben
// sincronizarse o exponerse a los workers subordinados para evitar delegación recursiva
// o bucles de orquestación.
var BlockedWorkerMCPServers = []string{"cliostra"}

// IsAllowedWorkerMCPServer valida si una entrada de servidor MCP es segura para
// que un worker la utilice (Engram, Context7, CodeGraph están permitidos; Cliostra está bloqueado).
func IsAllowedWorkerMCPServer(name string, entry mcpServerEntry) bool {
	lowerName := strings.ToLower(strings.TrimSpace(name))
	for _, blocked := range BlockedWorkerMCPServers {
		if lowerName == blocked || strings.HasPrefix(lowerName, blocked+"-") || strings.HasSuffix(lowerName, "-"+blocked) {
			return false
		}
	}
	lowerCmd := strings.ToLower(strings.TrimSpace(entry.Command))
	if strings.Contains(lowerCmd, "cliostra") {
		return false
	}
	return true
}
