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

type ResponseAPIResponse struct {
	ID     string `json:"id"`
	Output []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"output"`
}

type Response struct {
	Response  string   `json:"response"`
	Codes     []string `json:"codes"`
	ShowCodes bool     `json:"show_codes"`
}

// Ask sends a message to the assistant via Response API without SDK
func (o *Overseer) Ask(user *entity.User, userMsg string, assistant entity.Assistant) (string, []entity.ProductInfo, error) {
	defer func() {
		if r := recover(); r != nil {
			o.log.With(slog.Any("panic", r)).Error("panic caught in Ask")
			o.locker.Unlock(user.UUID) // ensure unlock
		}
	}()

	apiKey := o.apiKey

	reqBody := ResponseAPIRequest{
		Model: assistant.Model,
		Input: []MessageItem{
			{
				Role: "developer",
				Content: []ContentItem{
					{
						Type: "input_text",
						Text: assistant.Prompt,
					},
				},
			},
			{
				Role: "user",
				Content: []ContentItem{
					{
						Type: "input_text",
						Text: userMsg,
					},
				},
			},
		},
		Text: TextSchema{
			Format: Format{
				Type:   "json_schema",
				Name:   "response_schema",
				Strict: true,
				Schema: entity.GetResponseFormat(assistant.ResponseFormat),
			},
			Verbosity: "medium",
		},
		Reasoning: Reasoning{Effort: "medium", Summary: "auto"},
		Tools: []Tool{
			{
				Type:           "file_search",
				VectorStoreIDs: []string{assistant.VectorStoreId},
			},
			{
				Type:            "mcp",
				ServerLabel:     "darkcs",
				ServerURL:       "https://backup.darkbyrior.com/api/v1/mcp",
				Headers:         map[string]string{"Authorization": "Bearer " + o.mcpKey},
				AllowedTools:    assistant.AllowedTools,
				RequireApproval: "never",
			},
		},
		PreviousResponseID: user.PrevRespID,
		Store:              true,
	}

	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(context.Background(), "POST", "https://api.openai.com/v1/responses", bytes.NewBuffer(b))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	// Read the full body safely (limit to 10MB to avoid OOM)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return "", nil, fmt.Errorf("failed to read response body: %v", err)
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
		return "", nil, fmt.Errorf("response API error: %s", string(body))
	}

	// Unmarshal the body that was already read
	var apiResp ResponseAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", nil, fmt.Errorf("failed to decode response body: %v", err)
	}

	// Safety check
	if len(apiResp.Output) == 0 || apiResp.Output[0].Text == "" {
		return "", nil, fmt.Errorf("no output from assistant")
	}

	// Parse assistant JSON response safely
	var r Response
	if err := json.Unmarshal([]byte(apiResp.Output[0].Text), &r); err != nil {
		o.log.With(
			slog.String("userUUID", user.UUID),
			slog.Any("response", apiResp.Output[0].Text),
			sl.Err(err),
		).Error("unmarshalling response")
		return apiResp.Output[0].Text, nil, nil
	}

	// Clean text
	r.Response = regexp.MustCompile(`【\d+:\d+†[^】]+】`).ReplaceAllString(r.Response, "")

	var products []entity.ProductInfo
	if r.ShowCodes && len(r.Codes) > 0 {
		products, _ = o.productService.GetProductInfo(r.Codes)
	}

	// Update user's previous response ID
	err = o.authService.SetPrevRespID(*user, apiResp.ID)
	if err != nil {
		o.log.With(
			slog.String("userUUID", user.UUID),
			sl.Err(err),
		).Error("setting previous response ID")
	}

	return r.Response, products, nil
}
