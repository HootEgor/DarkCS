package gpt

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
)

type ResponseAPIRequest struct {
	Model              string        `json:"model"`
	Input              []MessageItem `json:"input"`
	Text               TextSchema    `json:"text"`
	Reasoning          Reasoning     `json:"reasoning"`
	Tools              []Tool        `json:"tools"`
	PreviousResponseID string        `json:"previous_response_id,omitempty"`
	Store              bool          `json:"store"`
}

type MessageItem struct {
	Role    string        `json:"role"`
	Content []ContentItem `json:"content"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type TextSchema struct {
	Format    Format `json:"format"`
	Verbosity string `json:"verbosity"`
}

type Format struct {
	Type   string      `json:"type"`
	Name   string      `json:"name"`
	Strict bool        `json:"strict"`
	Schema interface{} `json:"schema"`
}

type Reasoning struct {
	Effort  string `json:"effort"`
	Summary string `json:"summary"`
}

type Tool struct {
	Type            string            `json:"type"`
	VectorStoreIDs  []string          `json:"vector_store_ids,omitempty"`
	ServerLabel     string            `json:"server_label,omitempty"`
	ServerURL       string            `json:"server_url,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	AllowedTools    []string          `json:"allowed_tools,omitempty"`
	RequireApproval string            `json:"require_approval,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type ResponseAPIResponse struct {
	ID     string `json:"id"`
	Output []struct {
		Type    string `json:"type"`
		Status  string `json:"status,omitempty"`
		Role    string `json:"role,omitempty"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content,omitempty"`
	} `json:"output"`
	Usage Usage `json:"usage"`
}

// Ask sends a message to the assistant via Response API without SDK
func (o *Overseer) Ask(user *entity.User, userMsg string, assistant entity.Assistant) (string, error) {
	defer func() {
		if r := recover(); r != nil {
			o.log.With(slog.Any("panic", r)).Error("panic caught in Ask")
			o.locker.Unlock(user.UUID) // ensure unlock
		}
	}()

	apiKey := o.apiKey

	var input []MessageItem
	var tools []Tool

	prevRespID := user.PrevRespID
	if prevRespID == "" || assistant.Name == entity.OverseerAss {
		prevRespID = ""
		// New conversation
		input = []MessageItem{
			{
				Role: "developer",
				Content: []ContentItem{
					{Type: "input_text", Text: assistant.Prompt},
				},
			},
			{
				Role: "user",
				Content: []ContentItem{
					{Type: "input_text", Text: userMsg},
				},
			},
		}
		tools = []Tool{}
		if len(assistant.VectorStoreId) > 2 {
			tools = append(tools, Tool{
				Type:           "vector_store",
				VectorStoreIDs: []string{assistant.VectorStoreId},
			})
		}
		if len(assistant.AllowedTools) > 0 {
			tools = append(tools, Tool{
				Type:        "mcp",
				ServerLabel: "darkcs",
				ServerURL:   "https://backup.darkbyrior.com/api/v1/mcp",
				Headers: map[string]string{
					"Authorization": "Bearer " + o.mcpKey,
					"X-Assistant":   assistant.Name,
					"X-User-UUID":   user.UUID,
				},
				AllowedTools:    assistant.AllowedTools,
				RequireApproval: "never",
			})
		}
	} else {
		// Follow-up
		input = []MessageItem{
			{
				Role: "user",
				Content: []ContentItem{
					{Type: "input_text", Text: userMsg},
				},
			},
		}
		tools = nil // omit tools
	}

	reqBody := ResponseAPIRequest{
		Model: assistant.Model,
		Input: input,
		Text: TextSchema{
			Format: Format{
				Type:   "json_schema",
				Name:   "response_schema",
				Strict: true,
				Schema: entity.GetResponseFormat(assistant.ResponseFormat),
			},
			Verbosity: "medium",
		},
		Reasoning:          Reasoning{Effort: "medium", Summary: "auto"},
		Tools:              tools,
		PreviousResponseID: prevRespID,
		Store:              true,
	}

	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(context.Background(), "POST", "https://api.openai.com/v1/responses", bytes.NewBuffer(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Read the full body safely (limit to 10MB to avoid OOM)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	// Log the response body for debugging
	previewLen := 2000
	if len(body) < previewLen {
		previewLen = len(body)
	}
	o.log.With(
		slog.String("body_preview", string(body[:previewLen])),
		slog.Int("body_length", len(body)),
	).Debug("full Response API body")
	for i := 0; i < len(body); i += 2000 {
		end := i + 2000
		if end > len(body) {
			end = len(body)
		}
		o.log.With(
			slog.String("body_chunk", string(body[i:end])),
			slog.Int("chunk_start", i),
			slog.Int("chunk_end", end),
		).Debug("Response API body chunk")
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("response API error: %s", string(body))
	}

	// Unmarshal the body that was already read
	var apiResp ResponseAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("failed to decode response body: %v", err)
	}

	assistantText := ""
	for _, out := range apiResp.Output {
		if out.Type == "message" {
			for _, c := range out.Content {
				if c.Type == "output_text" && c.Text != "" {
					assistantText = c.Text // keep overwriting → final one wins
				}
				o.log.With(
					slog.String("resp", c.Text),
				).Info("assistant response received")
			}
		}
	}

	if assistantText == "" {
		o.log.With(
			slog.String("userUUID", user.UUID),
			slog.String("responseID", apiResp.ID),
		).Warn("no assistant message found in Response API output")
		return string(body), fmt.Errorf("no output from assistant")
	}

	const contextLimit = 400000
	const safeMargin = int(float64(contextLimit) * 0.9) // ~360k

	if assistant.Name != entity.OverseerAss {
		if apiResp.Usage.InputTokens > safeMargin {
			o.log.With(
				slog.String("userUUID", user.UUID),
				slog.Int("input_tokens", apiResp.Usage.InputTokens),
			).Info("context window near limit, resetting conversation")
			// Don’t save PrevRespID → new conversation next time
			err = o.authService.SetPrevRespID(*user, "")
		} else {
			err = o.authService.SetPrevRespID(*user, apiResp.ID)
		}

		if err != nil {
			o.log.With(
				slog.String("userUUID", user.UUID),
				sl.Err(err),
			).Error("setting previous response ID")
		}
	}

	return assistantText, nil
}

func (o *Overseer) getResponse(user *entity.User, userMsg string, assistant entity.Assistant) (string, []entity.ProductInfo, error) {
	response, err := o.Ask(user, userMsg, assistant)

	// Now you can safely unmarshal it
	var r entity.ResponseCode
	if err := json.Unmarshal([]byte(response), &r); err != nil {
		o.log.With(
			slog.String("userUUID", user.UUID),
			slog.Any("response", response),
			sl.Err(err),
		).Error("unmarshalling assistant response")
		return response, nil, fmt.Errorf("invalid response format")
	}

	// Clean text
	r.Response = regexp.MustCompile(`【\d+:\d+†[^】]+】`).ReplaceAllString(r.Response, "")

	var products []entity.ProductInfo
	if r.ShowCodes && len(r.Codes) > 0 {
		products, _ = o.productService.GetProductInfo(r.Codes)
	}

	return r.Response, products, err
}

func (o *Overseer) determineAssistant(user *entity.User, systemMsg, userMsg string) (string, error) {
	question := fmt.Sprintf("%s, HttpUserMsg: %s", systemMsg, userMsg)

	assistant, err := o.repo.GetAssistant(entity.OverseerAss)
	if err != nil {
		o.log.With(
			slog.String("assistant", entity.OverseerAss),
			slog.String("userUUID", user.UUID),
		).Error("get assistant", sl.Err(err))
		return "", fmt.Errorf("failed to get assistant %s: %v", entity.OverseerAss, err)
	}

	response, err := o.Ask(user, question, *assistant)
	if err != nil {
		return "", err
	}

	var r entity.ResponseAssistant
	if err := json.Unmarshal([]byte(response), &r); err != nil {
		o.log.With(
			slog.String("userUUID", user.UUID),
			slog.Any("response", response),
			sl.Err(err),
		).Error("unmarshalling assistant response")
		return response, fmt.Errorf("invalid response format")
	}

	return r.Assistant, nil
}
