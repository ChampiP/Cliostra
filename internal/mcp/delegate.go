package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ChampiP/Cliostra/internal/api"
	"github.com/ChampiP/Cliostra/internal/notify"
)

// delegateWaitTimeout es el tope de espera en la goroutine de fondo de
// "delegate". No bloquea a nadie (ver nota sobre context.Background() más
// abajo), así que puede ser mucho más generoso que maxWaitTimeout.
const delegateWaitTimeout = 2 * time.Hour

// diffNotifyLimit acota cuántos caracteres del diff se incluyen en el aviso,
// para no mandar potenciales megabytes al socket de mensajería.
const diffNotifyLimit = 4000

type delegateArgs struct {
	Adapter  string `json:"adapter" jsonschema:"nombre del adaptador (claude-code o agy)"`
	Repo     string `json:"repo" jsonschema:"ruta absoluta al repositorio git"`
	Prompt   string `json:"prompt" jsonschema:"instrucción para el trabajo"`
	ReadOnly bool   `json:"read_only" jsonschema:"true: solo inspecciona, nunca edita. false: edita y ejecuta comandos de verdad, pero SOLO dentro de un worktree git desechable (detached HEAD); el repo real nunca se toca. El aviso final incluye el diff para revisar antes de aplicarlo a tu rama real."`
	Model    string `json:"model,omitempty" jsonschema:"modelo a usar, opcional: si se omite se usa el default del CLI del adaptador."`
	Effort   string `json:"effort,omitempty" jsonschema:"nivel de esfuerzo a usar, opcional: si se omite se usa el default del CLI del adaptador (ej. low, medium, high)."`
}

const delegateDescription = "Delega un trabajo de forma ASÍNCRONA: lo encola y devuelve el control de inmediato con su id, SIN esperar a que termine (a diferencia de \"run\", que bloquea hasta el resultado). El resultado va a llegar solo, más adelante, como un mensaje NUEVO en esta misma sesión cuando el trabajo termine — no hace falta ni corresponde consultar \"status\" o \"result\" a propósito, ni prometerle al usuario que se le va a avisar cuando termine: el aviso ya es automático y no depende de que sigas la conversación. Solo funciona cuando Cliostra corre como servidor MCP dentro de una sesión de Claude Code (necesita su socket de mensajería); si no, esta tool falla de entrada y hay que usar \"run\" en su lugar."

// addDelegateTool registra la tool "delegate", el equivalente asíncrono de
// "run": arranca el trabajo y devuelve el id sin esperar, y notifica el
// resultado más tarde vía internal/notify desde una goroutine de fondo.
func addDelegateTool(s *sdk.Server, dial Dialer, notifyCfg notify.Config) {
	sdk.AddTool(s, &sdk.Tool{
		Name:        "delegate",
		Description: delegateDescription,
	}, func(_ context.Context, _ *sdk.CallToolRequest, args delegateArgs) (*sdk.CallToolResult, api.StartResponse, error) {
		if !notifyCfg.Available() {
			err := errors.New("delegate solo funciona cuando Cliostra corre como servidor MCP dentro de una sesión de Claude Code (falta el socket de mensajería); usá \"run\" en su lugar")
			return toolResult(err), api.StartResponse{}, nil
		}

		var start api.StartResponse
		if err := call(dial, "start", api.StartRequest{
			Adapter: args.Adapter, Repo: args.Repo, Prompt: args.Prompt, ReadOnly: args.ReadOnly,
			Model: args.Model, Effort: args.Effort,
		}, &start); err != nil {
			return toolResult(err), api.StartResponse{}, nil
		}

		// Importante: NO se usa el ctx de la llamada de la tool, porque se
		// cancela apenas termina el turno actual, justo cuando esta espera
		// tiene que seguir viva. El plugin de OpenCode tuvo este mismo bug.
		go notifyOnCompletion(dial, notifyCfg, start.ID, args.Adapter)

		return toolResult(nil), start, nil
	})
}

// notifyOnCompletion espera el resultado terminal del trabajo y avisa a la
// sesión de Claude Code por el socket de mensajería. No hay nadie esperando
// esta goroutine, así que un error solo se descarta (no hay a quién
// reportarlo) y no se reintenta.
func notifyOnCompletion(dial Dialer, cfg notify.Config, id, adapter string) {
	resp, err := waitForResultCapped(context.Background(), dial, waitArgs{
		ID:             id,
		TimeoutSeconds: int(delegateWaitTimeout.Seconds()),
	}, delegateWaitTimeout)

	text := formatDelegateNotification(id, adapter, resp, err)
	_ = notify.Send(cfg, text)
}

// formatDelegateNotification arma el texto que se inyecta en la sesión:
// id, adaptador, estado final y resultado o motivo del fallo.
func formatDelegateNotification(id, adapter string, resp api.ResultResponse, err error) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Cliostra: el trabajo delegado %s (adaptador %s) terminó.\n", id, adapter)

	if err != nil {
		fmt.Fprintf(&b, "No se pudo obtener el resultado: %s\n", err)
		return b.String()
	}

	fmt.Fprintf(&b, "Estado: %s\n", resp.State)
	if !resp.Available {
		b.WriteString("El resultado todavía no está disponible (se agotó el tiempo de espera del lado del servidor); usá la tool \"result\" con este id para consultarlo más tarde.\n")
		return b.String()
	}

	if resp.Result != "" {
		fmt.Fprintf(&b, "Resultado:\n%s\n", resp.Result)
	}
	if resp.Diff != "" {
		diff := resp.Diff
		truncated := len(diff) > diffNotifyLimit
		if truncated {
			diff = diff[:diffNotifyLimit]
		}
		fmt.Fprintf(&b, "Diff:\n%s\n", diff)
		if truncated {
			b.WriteString("(diff truncado)\n")
		}
	}
	return b.String()
}
