package runtime

import (
	"encoding/json"

	"github.com/ChampiP/Cliostra/internal/api"
)

// Handlers construye el mapa de método RPC → Handler que cliostrad expone
// sobre el socket Unix, delegando toda la lógica al Runtime.
func Handlers(r *Runtime) map[string]api.Handler {
	return map[string]api.Handler{
		"start":  startHandler(r),
		"status": statusHandler(r),
		"result": resultHandler(r),
		"cancel": cancelHandler(r),
	}
}

func startHandler(r *Runtime) api.Handler {
	return func(payload json.RawMessage) (any, *api.Error) {
		var req api.StartRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, api.NewError(api.ErrInvalidArgument, err.Error())
		}
		id, err := r.Start(StartRequest{
			Adapter: req.Adapter, Repo: req.Repo, Prompt: req.Prompt, ReadOnly: req.ReadOnly,
			Model: req.Model, Effort: req.Effort,
		})
		if err != nil {
			return nil, api.NewError(api.ErrInvalidArgument, err.Error())
		}
		return api.StartResponse{ID: id}, nil
	}
}

func statusHandler(r *Runtime) api.Handler {
	return func(payload json.RawMessage) (any, *api.Error) {
		var req api.StatusRequest
		if err := json.Unmarshal(payload, &req); err != nil || req.ID == "" {
			return nil, api.NewError(api.ErrInvalidArgument, "id requerido")
		}
		j, err := r.Status(req.ID)
		if err != nil {
			return nil, api.NewError(api.ErrNotFound, err.Error())
		}
		return api.StatusResponse{
			ID: j.ID, State: string(j.State), Reason: j.Reason, Truncated: j.Truncated,
		}, nil
	}
}

func resultHandler(r *Runtime) api.Handler {
	return func(payload json.RawMessage) (any, *api.Error) {
		var req api.ResultRequest
		if err := json.Unmarshal(payload, &req); err != nil || req.ID == "" {
			return nil, api.NewError(api.ErrInvalidArgument, "id requerido")
		}
		j, err := r.Status(req.ID)
		if err != nil {
			return nil, api.NewError(api.ErrNotFound, err.Error())
		}
		if !j.State.IsTerminal() {
			return api.ResultResponse{ID: j.ID, State: string(j.State), Available: false}, nil
		}
		return api.ResultResponse{
			ID: j.ID, State: string(j.State), Available: true, Result: j.Result, Diff: j.Diff, Truncated: j.Truncated,
		}, nil
	}
}

func cancelHandler(r *Runtime) api.Handler {
	return func(payload json.RawMessage) (any, *api.Error) {
		var req api.CancelRequest
		if err := json.Unmarshal(payload, &req); err != nil || req.ID == "" {
			return nil, api.NewError(api.ErrInvalidArgument, "id requerido")
		}
		j, supported, err := r.Cancel(req.ID)
		if err != nil {
			return nil, api.NewError(api.ErrNotFound, err.Error())
		}
		return api.CancelResponse{ID: j.ID, State: string(j.State), Supported: supported}, nil
	}
}
