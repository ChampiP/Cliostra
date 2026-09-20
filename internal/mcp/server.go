// Paquete mcp expone run/wait/status/result/cancel como herramientas MCP que
// delegan íntegramente al cliente RPC de internal/api: no reimplementan
// lógica de negocio. "start" (arrancar sin esperar) existe solo como método
// RPC interno, usado por la CLI y por "run"; no se expone como tool MCP a
// propósito, porque el modelo delegador tiende a llamarlo y quedarse
// esperando que alguien más le pida "status" después, en vez de esperar el
// resultado en la misma llamada.
package mcp

import (
	"context"
	"net"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ChampiP/Cliostra/internal/api"
	"github.com/ChampiP/Cliostra/internal/notify"
)

// waitPollInterval es el intervalo entre reintentos de "wait". Fijo y bajo:
// no hay push/notificación real, solo polling del lado del servidor MCP.
const waitPollInterval = 500 * time.Millisecond

// defaultWaitTimeout se usa cuando el llamador no especifica timeout_seconds.
const defaultWaitTimeout = 60 * time.Second

// maxWaitTimeout acota cuánto puede bloquear "wait" para no colgar al
// cliente MCP indefinidamente si el job nunca termina.
const maxWaitTimeout = 10 * time.Minute

// Dialer abre una conexión al socket Unix de cliostrad. Se inyecta para
// poder probar el servidor MCP con un transporte en memoria, sin proveedores
// reales.
type Dialer func() (net.Conn, error)

type idArgs struct {
	ID string `json:"id" jsonschema:"identificador del trabajo"`
}

type waitArgs struct {
	ID             string `json:"id" jsonschema:"identificador del trabajo"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"tiempo máximo de espera en segundos (default 60, máximo 600)"`
}

type runArgs struct {
	Adapter        string `json:"adapter" jsonschema:"nombre del adaptador (claude-code, agy, codex u opencode)"`
	Repo           string `json:"repo" jsonschema:"ruta absoluta al repositorio git"`
	Prompt         string `json:"prompt" jsonschema:"instrucción para el trabajo"`
	ReadOnly       bool   `json:"read_only" jsonschema:"true: instruye al proveedor a solo inspeccionar; no es aislamiento estructural. false: edita y ejecuta comandos de verdad. claude-code, agy, codex y opencode trabajan directamente sobre el repositorio compartido, por lo que los cambios locales se ven durante la ejecución. Usa git diff para inspeccionarlos en vivo."`
	Model          string `json:"model,omitempty" jsonschema:"modelo a usar, opcional: si se omite se usa el default del CLI del adaptador. Los nombres válidos dependen del adaptador, por ejemplo para claude-code: sonnet, opus; para agy: gemini-3.8-flash-low."`
	Effort         string `json:"effort,omitempty" jsonschema:"nivel de esfuerzo a usar, opcional: si se omite se usa el default del CLI del adaptador (ej. low, medium, high)."`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"tiempo máximo de espera del resultado en segundos (default 60, máximo 600)"`
}

// NewServer construye el servidor MCP con las herramientas del contrato
// (run/wait/status/result/cancel/delegate), todas delegando al RPC vía dial.
// notifyCfg se usa solo por "delegate" para avisar cuando un trabajo
// asíncrono termina; el llamador real la arma con notify.ConfigFromEnv(), y
// los tests pueden inyectar una config apuntando a un socket falso.
func NewServer(dial Dialer, notifyCfg notify.Config) *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{Name: "cliostra", Version: "0.1.0"}, nil)

	sdk.AddTool(s, &sdk.Tool{
		Name:        "run",
		Description: "Delega un trabajo y devuelve el resultado final ya resuelto, en una sola llamada (bloquea del lado del servidor hasta que termine o hasta timeout_seconds). Es la única forma de arrancar un trabajo nuevo: no existe un \"start\" separado a propósito, para no dejar trabajos arrancados sin que nadie espere su resultado.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, args runArgs) (*sdk.CallToolResult, api.ResultResponse, error) {
		var start api.StartResponse
		if err := call(dial, "start", api.StartRequest{
			Adapter: args.Adapter, Repo: args.Repo, Prompt: args.Prompt, ReadOnly: args.ReadOnly,
			Model: args.Model, Effort: args.Effort,
		}, &start); err != nil {
			return toolResult(err), api.ResultResponse{}, nil
		}
		resp, err := waitForResult(ctx, dial, waitArgs{ID: start.ID, TimeoutSeconds: args.TimeoutSeconds})
		return toolResult(err), resp, nil
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "status",
		Description: "Devuelve el estado durable actual de un trabajo.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, args idArgs) (*sdk.CallToolResult, api.StatusResponse, error) {
		var resp api.StatusResponse
		err := call(dial, "status", api.StatusRequest{ID: args.ID}, &resp)
		return toolResult(err), resp, nil
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "result",
		Description: "Devuelve el resultado terminal de un trabajo, o indica que aún no está disponible.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, args idArgs) (*sdk.CallToolResult, api.ResultResponse, error) {
		var resp api.ResultResponse
		err := call(dial, "result", api.ResultRequest{ID: args.ID}, &resp)
		return toolResult(err), resp, nil
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "wait",
		Description: "Retoma la espera de un trabajo que ya tiene id (por ejemplo, porque `run` agotó su timeout y el job sigue corriendo). Bloquea hasta que termine o se agote timeout_seconds.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, args waitArgs) (*sdk.CallToolResult, api.ResultResponse, error) {
		resp, err := waitForResult(ctx, dial, args)
		return toolResult(err), resp, nil
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "cancel",
		Description: "Solicita cancelar un trabajo; informa no compatible si el adaptador no la soporta.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, args idArgs) (*sdk.CallToolResult, api.CancelResponse, error) {
		var resp api.CancelResponse
		err := call(dial, "cancel", api.CancelRequest{ID: args.ID}, &resp)
		return toolResult(err), resp, nil
	})

	addDelegateTool(s, dial, notifyCfg)

	return s
}

// waitForResult sondea "result" hasta que el trabajo termine, se agote el
// timeout, o el contexto de la llamada se cancele. Acota el timeout a
// maxWaitTimeout porque bloquea al cliente MCP que llamó a "wait"/"run".
func waitForResult(ctx context.Context, dial Dialer, args waitArgs) (api.ResultResponse, error) {
	return waitForResultCapped(ctx, dial, args, maxWaitTimeout)
}

// waitForResultCapped es el núcleo compartido de espera, parametrizado por
// el tope de timeout: "wait"/"run" usan maxWaitTimeout porque bloquean al
// cliente MCP, mientras que "delegate" espera en una goroutine de fondo y
// puede usar un tope mucho más generoso (ver delegate.go).
func waitForResultCapped(ctx context.Context, dial Dialer, args waitArgs, cap time.Duration) (api.ResultResponse, error) {
	timeout := defaultWaitTimeout
	if args.TimeoutSeconds > 0 {
		timeout = time.Duration(args.TimeoutSeconds) * time.Second
	}
	if timeout > cap {
		timeout = cap
	}
	deadline := time.Now().Add(timeout)

	for {
		var resp api.ResultResponse
		if err := call(dial, "result", api.ResultRequest{ID: args.ID}, &resp); err != nil {
			return api.ResultResponse{}, err
		}
		if resp.Available {
			return resp, nil
		}
		if !time.Now().Before(deadline) {
			return resp, nil
		}
		select {
		case <-ctx.Done():
			return resp, ctx.Err()
		case <-time.After(waitPollInterval):
		}
	}
}

func call(dial Dialer, method string, req, resp any) error {
	conn, err := dial()
	if err != nil {
		return err
	}
	defer conn.Close()
	return api.Call(conn, method, req, resp)
}

// toolResult marca IsError cuando la llamada RPC falló, sin romper el
// contrato MCP (la herramienta siempre responde, nunca bloquea al cliente).
func toolResult(err error) *sdk.CallToolResult {
	if err == nil {
		return &sdk.CallToolResult{}
	}
	return &sdk.CallToolResult{
		IsError: true,
		Content: []sdk.Content{&sdk.TextContent{Text: err.Error()}},
	}
}
