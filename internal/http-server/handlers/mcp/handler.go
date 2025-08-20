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

// Example MCP handler over HTTP
func Handler(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Read body
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		// Restore the body so it can be decoded later
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
		case "ping":
			pong := handler.Ping()
			res.Result = map[string]string{"msg": pong}
		case "get_products_info":
			// Parse params
			var params struct {
				Codes []string `json:"codes"`
			}
			if err := json.Unmarshal(req.Params, &params); err != nil {
				res.Error = "invalid params: " + err.Error()
				break
			}

			products, err := handler.ProductsInfo(params.Codes)
			if err != nil {
				res.Error = err.Error()
				break
			}

			res.Result = map[string]interface{}{
				"products": products,
			}
		default:
			res.Error = "unknown method: " + req.Method
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	}
}
