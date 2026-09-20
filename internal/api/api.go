// Paquete api define los DTO, códigos de error y el framing RPC en JSON-lines
// usado por cliostrad, la CLI y el servidor MCP para comunicarse sobre un
// socket Unix privado.
package api

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
)

// MaxFrameSize es el límite duro por frame (256 KiB) para evitar que un
// mensaje malformado o adversarial agote memoria del demonio.
const MaxFrameSize = 256 * 1024

// Código de error estable para que CLI y MCP mapeen mensajes sin parsear texto.
type ErrorCode string

const (
	ErrNotFound        ErrorCode = "not_found"
	ErrInvalidArgument ErrorCode = "invalid_argument"
	ErrUnsupported     ErrorCode = "unsupported"
	ErrInternal        ErrorCode = "internal"
	ErrFrameTooLarge   ErrorCode = "frame_too_large"
)

// Error es el error estructurado que viaja en la respuesta RPC.
type Error struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewError construye un *Error listo para devolver o serializar.
func NewError(code ErrorCode, msg string) *Error {
	return &Error{Code: code, Message: msg}
}

// StartRequest es la solicitud para iniciar un trabajo.
type StartRequest struct {
	Adapter  string `json:"adapter"`
	Repo     string `json:"repo"`
	Prompt   string `json:"prompt"`
	ReadOnly bool   `json:"read_only"`
	Model    string `json:"model,omitempty"`
	Effort   string `json:"effort,omitempty"`
}

// StartResponse confirma el identificador asignado sin esperar finalización.
type StartResponse struct {
	ID string `json:"id"`
}

// StatusRequest identifica el trabajo a consultar.
type StatusRequest struct {
	ID string `json:"id"`
}

// StatusResponse refleja el estado durable actual.
type StatusResponse struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	Reason    string `json:"reason,omitempty"`
	Truncated bool   `json:"truncated"`
}

// ResultRequest identifica el trabajo cuyo resultado se solicita.
type ResultRequest struct {
	ID string `json:"id"`
}

// ResultResponse entrega el resultado terminal o indica que aún no está listo.
type ResultResponse struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Result    string `json:"result,omitempty"`
	Diff      string `json:"diff,omitempty"`
	Truncated bool   `json:"truncated"`
}

// CancelRequest identifica el trabajo a cancelar.
type CancelRequest struct {
	ID string `json:"id"`
}

// CancelResponse comunica fielmente el desenlace de la cancelación.
type CancelResponse struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	Supported bool   `json:"supported"`
}

// envelope es el frame único que viaja en ambas direcciones del socket.
type envelope struct {
	Method  string          `json:"method,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// WriteFrame serializa v como una línea JSON terminada en '\n', rechazando
// frames que excedan MaxFrameSize.
func WriteFrame(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b)+1 > MaxFrameSize {
		return NewError(ErrFrameTooLarge, "frame excede el límite de 256KiB")
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

// ReadFrame lee una línea JSON del reader, acotando el tamaño leído a
// MaxFrameSize para no permitir agotamiento de memoria por un peer hostil.
// El *bufio.Reader debe reutilizarse durante toda la vida de la conexión:
// crear uno nuevo por llamada perdería bytes ya bufferizados.
func ReadFrame(r *bufio.Reader, v any) error {
	var buf []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			if err == io.EOF && len(buf) == 0 {
				return io.EOF
			}
			if err == io.EOF {
				break
			}
			return err
		}
		if b == '\n' {
			break
		}
		buf = append(buf, b)
		if len(buf) > MaxFrameSize {
			return NewError(ErrFrameTooLarge, "frame excede el límite de 256KiB")
		}
	}
	return json.Unmarshal(buf, v)
}

// Call envía method+req por conn y decodifica la respuesta en resp. Si el
// servidor devuelve un Error estructurado, Call lo retorna como error.
func Call(conn net.Conn, method string, req any, resp any) error {
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err := WriteFrame(conn, envelope{Method: method, Payload: payload}); err != nil {
		return err
	}
	reader := bufio.NewReader(conn)
	var respEnv envelope
	if err := ReadFrame(reader, &respEnv); err != nil {
		return err
	}
	if respEnv.Error != nil {
		return respEnv.Error
	}
	if resp == nil || len(respEnv.Payload) == 0 {
		return nil
	}
	return json.Unmarshal(respEnv.Payload, resp)
}

// Handler procesa el payload de un método y devuelve la respuesta o un error
// estructurado.
type Handler func(payload json.RawMessage) (any, *Error)

// Serve atiende una única conexión hasta que el peer la cierre o ocurra un
// error de protocolo. No bloquea otras conexiones: se espera invocar por
// goroutine desde el llamador.
func Serve(conn net.Conn, handlers map[string]Handler) error {
	reader := bufio.NewReader(conn)
	for {
		var req envelope
		if err := ReadFrame(reader, &req); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		h, ok := handlers[req.Method]
		if !ok {
			_ = WriteFrame(conn, envelope{Error: NewError(ErrInvalidArgument, "método desconocido: "+req.Method)})
			continue
		}
		resp, apiErr := h(req.Payload)
		if apiErr != nil {
			if err := WriteFrame(conn, envelope{Error: apiErr}); err != nil {
				return err
			}
			continue
		}
		payload, err := json.Marshal(resp)
		if err != nil {
			_ = WriteFrame(conn, envelope{Error: NewError(ErrInternal, err.Error())})
			continue
		}
		if err := WriteFrame(conn, envelope{Payload: payload}); err != nil {
			return err
		}
	}
}
