// Paquete notify envía avisos a una sesión de Claude Code a través del
// socket Unix de mensajería que Claude Code inyecta en sus procesos hijos
// (CLAUDE_CODE_MESSAGING_SOCKET / CLAUDE_CODE_MESSAGING_TOKEN). Se usa para
// avisar cuando un trabajo delegado de forma asíncrona (tool "delegate")
// termina, sin que nadie tenga que consultar "status" a propósito.
package notify

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"
)

// writeTimeout acota cuánto puede tardar la escritura en el socket, para no
// colgarse si el socket está muerto o nadie lo está leyendo.
const writeTimeout = 5 * time.Second

// Config son los datos necesarios para notificar a una sesión de Claude
// Code. Se reciben explícitos (nunca os.Getenv en el medio de la lógica) para
// poder testear con un socket falso.
type Config struct {
	SocketPath string
	Token      string
}

// ConfigFromEnv lee la configuración de las variables de entorno que Claude
// Code inyecta en sus procesos hijos. Es el único punto del paquete que toca
// el entorno real.
func ConfigFromEnv() Config {
	return Config{
		SocketPath: os.Getenv("CLAUDE_CODE_MESSAGING_SOCKET"),
		Token:      os.Getenv("CLAUDE_CODE_MESSAGING_TOKEN"),
	}
}

// Available indica si corremos dentro de una sesión de Claude Code: el
// socket solo existe cuando el proceso fue lanzado por Claude Code.
func (c Config) Available() bool {
	return c.SocketPath != ""
}

type authMessage struct {
	Type  string `json:"type"`
	Token string `json:"token"`
}

// userMessage sigue el protocolo documentado por el propio Claude Code (ver
// su mensaje de ayuda "[uds-messaging] Inject messages"): type "user" con el
// contenido anidado en "message", no "user_message" con "content" plano.
type userMessage struct {
	Type    string             `json:"type"`
	Message userMessagePayload `json:"message"`
}

type userMessagePayload struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Send abre el socket de mensajería, envía la línea de autenticación (solo
// si hay token) y la línea de mensaje de usuario, y cierra. El protocolo es
// orientado a líneas (un JSON por línea, sin framing HTTP) y no responde
// nada: no hay ack que esperar.
func Send(cfg Config, text string) error {
	if !cfg.Available() {
		return fmt.Errorf("notify: no hay socket de mensajería de Claude Code configurado")
	}

	conn, err := net.DialTimeout("unix", cfg.SocketPath, writeTimeout)
	if err != nil {
		return fmt.Errorf("notify: no se pudo conectar a %s: %w", cfg.SocketPath, err)
	}
	defer conn.Close()

	if err := conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return fmt.Errorf("notify: no se pudo fijar el timeout de escritura: %w", err)
	}

	if cfg.Token != "" {
		if err := writeLine(conn, authMessage{Type: "auth", Token: cfg.Token}); err != nil {
			return fmt.Errorf("notify: fallo al enviar auth: %w", err)
		}
	}
	if err := writeLine(conn, userMessage{Type: "user", Message: userMessagePayload{Role: "user", Content: text}}); err != nil {
		return fmt.Errorf("notify: fallo al enviar el mensaje: %w", err)
	}
	return nil
}

func writeLine(conn net.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = conn.Write(b)
	return err
}
