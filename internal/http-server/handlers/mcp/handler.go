package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

// JSON-RPC request/response types
type RPCRequest struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type RPCResponse struct {
	Jsonrpc string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Example MCP handler over HTTP
func Handler(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		log.With(
			slog.String("module", "http.handlers.mcp"),
			slog.String("request", r.URL.Path),
			slog.String("body", string(bodyBytes)),
		).Debug("handling MCP request")

		var req RPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		res := RPCResponse{Jsonrpc: "2.0", ID: req.ID}

		switch req.Method {
		case "initialize":
			res.Result = map[string]interface{}{
				"protocolVersion": "2025-03-26",
				"serverInfo": map[string]interface{}{
					"name":    "Go MCP Server",
					"version": "1.0.0",
				},
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{
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
					res.Error = &ErrorResponse{Code: -32602, Message: "Invalid input for get_products_info: " + err.Error()}
					break
				}

				products, err := handler.ProductsInfo(params.Codes)
				if err != nil {
					res.Error = &ErrorResponse{Code: -32603, Message: err.Error()}
					break
				}

				res.Result = products
			default:
				res.Error = &ErrorResponse{Code: -32601, Message: "Tool not found: " + callParams.Name}
			}
		default:
			res.Error = &ErrorResponse{Code: -32601, Message: "Method not found: " + req.Method}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(res); err != nil {
			log.Error("failed to encode response", slog.Any("error", err))
		}
	}
}
