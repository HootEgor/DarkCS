package mcp

import (
	"encoding/json"
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
func MCPHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	res := RPCResponse{Jsonrpc: "2.0", ID: req.ID}

	switch req.Method {
	case "ping":
		res.Result = map[string]string{"msg": "pong"}
	case "echo":
		var input map[string]interface{}
		if err := json.Unmarshal(req.Params, &input); err != nil {
			res.Error = err.Error()
		} else {
			res.Result = map[string]interface{}{"echo": input}
		}
	default:
		res.Error = "unknown method: " + req.Method
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
