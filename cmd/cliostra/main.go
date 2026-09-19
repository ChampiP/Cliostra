// cliostra es el cliente: CLI con subcomandos start/status/result/cancel
// (cliente RPC sobre el socket de cliostrad) y un modo `mcp` que expone el
// servidor MCP por stdio.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ChampiP/Cliostra/internal/api"
	"github.com/ChampiP/Cliostra/internal/daemonpath"
	cliostramcp "github.com/ChampiP/Cliostra/internal/mcp"
	"github.com/ChampiP/Cliostra/internal/notify"
)

func dial() (net.Conn, error) {
	return net.Dial("unix", daemonpath.SocketPath())
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "start":
		cmdStart(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	case "result":
		cmdResult(os.Args[2:])
	case "cancel":
		cmdCancel(os.Args[2:])
	case "mcp":
		cmdMCP()
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "uso: cliostra <start|status|result|cancel|mcp> [flags]")
}

func printJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error de codificación:", err)
		os.Exit(1)
	}
	fmt.Println(string(b))
}

func mustDial() net.Conn {
	conn, err := dial()
	if err != nil {
		fmt.Fprintln(os.Stderr, "no se pudo conectar a cliostrad:", err)
		os.Exit(1)
	}
	return conn
}

func cmdStart(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	adapter := fs.String("adapter", "", "adaptador (claude-code, agy, codex u opencode)")
	repo := fs.String("repo", "", "ruta absoluta al repositorio git")
	prompt := fs.String("prompt", "", "instrucción para el trabajo")
	write := fs.Bool("write", false, "permite edición real: worktree administrado para claude-code/opencode, repo real directo para codex/agy (default: solo lectura)")
	model := fs.String("model", "", "modelo a usar (opcional; vacío = default del adaptador)")
	effort := fs.String("effort", "", "nivel de esfuerzo (opcional; vacío = default del adaptador)")
	fs.Parse(args)

	conn := mustDial()
	defer conn.Close()
	var resp api.StartResponse
	req := api.StartRequest{Adapter: *adapter, Repo: *repo, Prompt: *prompt, ReadOnly: !*write, Model: *model, Effort: *effort}
	if err := api.Call(conn, "start", req, &resp); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	printJSON(resp)
}

func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	id := fs.String("id", "", "identificador del trabajo")
	fs.Parse(args)

	conn := mustDial()
	defer conn.Close()
	var resp api.StatusResponse
	if err := api.Call(conn, "status", api.StatusRequest{ID: *id}, &resp); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	printJSON(resp)
}

func cmdResult(args []string) {
	fs := flag.NewFlagSet("result", flag.ExitOnError)
	id := fs.String("id", "", "identificador del trabajo")
	fs.Parse(args)

	conn := mustDial()
	defer conn.Close()
	var resp api.ResultResponse
	if err := api.Call(conn, "result", api.ResultRequest{ID: *id}, &resp); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	printJSON(resp)
}

func cmdCancel(args []string) {
	fs := flag.NewFlagSet("cancel", flag.ExitOnError)
	id := fs.String("id", "", "identificador del trabajo")
	fs.Parse(args)

	conn := mustDial()
	defer conn.Close()
	var resp api.CancelResponse
	if err := api.Call(conn, "cancel", api.CancelRequest{ID: *id}, &resp); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	printJSON(resp)
}

func cmdMCP() {
	server := cliostramcp.NewServer(dial, notify.ConfigFromEnv())
	if err := server.Run(context.Background(), &sdk.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, "servidor MCP terminado con error:", err)
		os.Exit(1)
	}
}
