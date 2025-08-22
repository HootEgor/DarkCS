package mcp

import (
	"encoding/json"
	"github.com/go-chi/render"
	"io"
	"log/slog"
	"net/http"
)

// JSON-RPC request/response types
type RPCRequest struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type RPCResponse struct {
	Jsonrpc string         `json:"jsonrpc"`
	ID      interface{}    `json:"id"`
	Result  interface{}    `json:"result,omitempty"`
	Error   *ErrorResponse `json:"error,omitempty"`
}

type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Example MCP handler over HTTP
func Handler(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		log.With(
			slog.String("module", "http.handlers.mcp"),
			slog.String("request", r.URL.Path),
			slog.String("body", string(bodyBytes)),
		).Debug("handling MCP request")

		var req RPCRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		res := RPCResponse{Jsonrpc: "2.0", ID: req.ID}

		if req.Method == "notifications/initialized" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		switch req.Method {
		case "initialize":
			res.Result = map[string]interface{}{
				"protocolVersion": "2025-06-18",
				"serverInfo": map[string]interface{}{
					"name":    "Go MCP Server",
					"version": "1.0.0",
				},
				// This capabilities block is the critical change.
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{
						// Explicitly state the tool methods this server supports.
						// This is the piece of information the client is depending on.
						"methods": []string{"list", "call"},

						// You can still indicate that the list is dynamic.
						"listChanged": true,
					},
				},
			}
		case "tools/list":
			res.Result = ToolsDescription()
		case "ping":
			pong := handler.Ping()
			res.Result = map[string]string{"msg": pong}
		case "tools/call":
			var callParams struct {
				Name  string          `json:"name"`
				Input json.RawMessage `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &callParams); err != nil {
				res.Error = &ErrorResponse{Code: -32602, Message: "Invalid params: " + err.Error()}
				break
			}

			switch callParams.Name {
			case "get_products_info":
				var params struct {
					Codes []string `json:"codes"`
				}
				if err := json.Unmarshal(callParams.Input, &params); err != nil {
					log.Error("failed to unmarshal get_products_info params", slog.Any("error", err))
					res.Error = &ErrorResponse{Code: -32602, Message: "Invalid input for get_products_info: " + err.Error()}
					break
				}

				products, err := handler.ProductsInfo(params.Codes)
				if err != nil {
					log.Error("failed to fetch products info", slog.Any("error", err))
					res.Error = &ErrorResponse{Code: -32603, Message: err.Error()}
					break
				}

				res.Result = map[string]interface{}{
					"products": products,
				}
			default:
				res.Error = &ErrorResponse{Code: -32601, Message: "Tool not found: " + callParams.Name}
			}
		default:
			res.Error = &ErrorResponse{Code: -32601, Message: "Method not found: " + req.Method}
		}

		//w.Header().Set("Content-Type", "application/json; charset=utf-8")
		//w.WriteHeader(http.StatusOK)
		//if err := json.NewEncoder(w).Encode(res); err != nil {
		//	log.Error("failed to encode response", slog.Any("error", err))
		//	http.Error(w, "failed to encode response", http.StatusInternalServerError)
		//	return
		//}
		render.JSON(w, r, res)
	}
}
