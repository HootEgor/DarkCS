package mcp

import (
	"DarkCS/entity"
	"encoding/json"
	"fmt"
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

// maxBodyBytes caps a JSON-RPC request; tool arguments are small.
const maxBodyBytes = 1 << 20

// Handler serves MCP JSON-RPC for OpenAI's hosted MCP tool. Request bodies are not
// logged: tool arguments and results contain customer PII.
func Handler(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		bodyBytes, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

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

		assistantName := r.Header.Get("X-Assistant")
		if assistantName == "" {
			assistantName = entity.ConsultantAss
		}

		userUUID := r.Header.Get("X-User-UUID")

		switch req.Method {
		case "initialize":
			res.Result = map[string]interface{}{
				"protocolVersion": "2025-06-18",
				"serverInfo": map[string]interface{}{
					"name":    "darkcs",
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
			res.Result = ToolsDescription(assistantName)
		case "tools/call":
			var callParams struct {
				Name  string          `json:"name"`
				Input json.RawMessage `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &callParams); err != nil {
				res.Error = &ErrorResponse{Code: -32602, Message: "Invalid params: " + err.Error()}
				break
			}
			if userUUID == "" {
				res.Error = &ErrorResponse{Code: -32602, Message: "X-User-UUID header is required"}
				break
			}
			if !ToolAllowed(assistantName, callParams.Name) {
				log.Warn("mcp tool not allowed for assistant",
					slog.String("assistant", assistantName), slog.String("tool", callParams.Name))
				res.Error = &ErrorResponse{Code: -32601, Message: "Tool not available: " + callParams.Name}
				break
			}

			cmdResp, err := handler.HandleCommand(userUUID, callParams.Name, callParams.Input)
			if err != nil {
				res.Error = &ErrorResponse{Code: -32603, Message: err.Error()}
				break
			}

			respText, err := json.MarshalIndent(cmdResp, "", "  ")
			if err != nil {
				respText = []byte(fmt.Sprintf("Failed to serialize response: %v", err))
			}

			res.Result = map[string]interface{}{
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": string(respText), // simple string display
					},
				},
				"isError": false,
			}

			//b, err := json.Marshal(cmdResp)
			//if err != nil {
			//	res.Result = map[string]interface{}{
			//		"isError": true,
			//		"content": []map[string]interface{}{
			//			{"type": "text", "text": "Unsupported response type"},
			//		},
			//	}
			//} else {
			//	var parsed interface{}
			//	_ = json.Unmarshal(b, &parsed)
			//
			//	res.Result = map[string]interface{}{
			//		"content": []map[string]interface{}{
			//			{
			//				"type": "text",
			//				"text": "Structured response",
			//			},
			//		},
			//		"structuredContent": map[string]interface{}{
			//			"data": parsed, // always wrap array in an object
			//		},
			//		"isError": false,
			//	}
			//}

			//res.Result = map[string]interface{}{
			//	"content": []map[string]interface{}{
			//		{
			//			"type": "output_text", // <-- FIXED
			//			"text": fmt.Sprintf("%v", cmdResp),
			//		},
			//	},
			//}
		default:
			res.Error = &ErrorResponse{Code: -32601, Message: "Method not found: " + req.Method}
		}

		render.JSON(w, r, res)
	}
}
