// Package gpt routes customer messages to specialized OpenAI assistants.
//
// Each message makes two OpenAI Responses API calls (raw HTTP, see response-ask.go): the
// Overseer picks an assistant, then that assistant answers. Conversation memory is the
// user's last Q/A pairs stored on the user document (auth.UpdateConversation), not
// OpenAI threads. Tools are served by our MCP endpoint, which OpenAI calls back.
// go-openai is still used for Whisper and for the nightly product vector stores.
//
// Files: overseer.go (types, routing, vector stores), response-ask.go (Responses API),
// cmd-handler.go (MCP tool implementations), audio.go (voice transcription).
package gpt

import (
	"DarkCS/entity"
	"DarkCS/internal/config"
	"DarkCS/internal/lib/sl"
	"context"
	"encoding/json"
	"fmt"
	_ "image/jpeg"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sashabaranov/go-openai"
)

type Repository interface {
	GetAssistant(name string) (*entity.Assistant, error)
	SetVectorStore(assistantName, vectorStoreID string) error
}

// ProductService defines the interface for product-related operations.
// It provides methods for retrieving product information, validating orders,
// and managing user discounts.
type ProductService interface {
	// GetProductInfo retrieves detailed information about products based on their article codes
	GetProductInfo(articles []string) ([]entity.ProductInfo, error)

	// GetAvailableProducts returns a list of all available products
	GetAvailableProducts() ([]entity.Product, error)

	// ValidateOrder validates an order's products and returns the validated products
	ValidateOrder([]entity.OrderProduct, string) ([]entity.OrderProduct, error)

	// GetUserDiscount retrieves the discount percentage for a user based on their phone number
	GetUserDiscount(phone string) (int, error)
}

// AuthService defines the interface for authentication and user-related operations.
// It provides methods for updating user information and managing shopping baskets.
type AuthService interface {
	// UpdateUserFields persists only the given user fields (entity.UserField* keys)
	UpdateUserFields(user *entity.User, fields map[string]any) error

	// UpdateBasket updates the contents of a user's shopping basket
	UpdateBasket(userUUID string, products []entity.OrderProduct) (*entity.Basket, error)

	// GetBasket retrieves the current contents of a user's shopping basket
	GetBasket(userUUID string) (*entity.Basket, error)

	// ClearBasket removes all products from a user's shopping basket
	ClearBasket(userUUID string) error

	// AddToBasket adds products to a user's shopping basket
	AddToBasket(userUUID string, products []entity.OrderProduct) (*entity.Basket, error)

	// RemoveFromBasket removes products from a user's shopping basket
	RemoveFromBasket(userUUID string, products []entity.OrderProduct) (*entity.Basket, error)

	UpdateConversation(user entity.User, conversation entity.DialogMessage) error
}

// ZohoService defines the interface for Zoho CRM integration.
// It provides methods for creating and retrieving orders.
type ZohoService interface {
	// CreateOrder creates a new order in the Zoho CRM system
	CreateOrder(order *entity.Order) error

	// GetOrders retrieves a list of orders for a specific user
	GetOrders(userInfo entity.UserInfo) ([]entity.OrderStatus, error)
}

// Overseer manages AI assistant interactions and coordinates with various services.
type Overseer struct {
	client         *openai.Client // OpenAI API client (Whisper, files, vector stores)
	apiKey         string         // OpenAI API key
	mcpKey         string         // MCP API key (mcp scope only)
	mcpURL         string         // public URL of this service's /api/v1/mcp
	productService ProductService // Service for product-related operations
	authService    AuthService    // Service for authentication and user operations
	zohoService    ZohoService    // Service for Zoho CRM integration
	repo           Repository
	savePath       string       // Path for saving files
	log            *slog.Logger // Logger instance

	// recentOrders remembers each user's last order for create_order idempotency.
	ordersMu     sync.Mutex
	recentOrders map[string]recentOrder
}

// maxUserMsgRunes caps a single customer message sent to OpenAI. Longer input is cut
// rather than rejected so existing callers keep working; it bounds per-message cost.
const maxUserMsgRunes = 4000

// citationMarker matches file_search citations like 【4:0†source】 that must not reach users.
var citationMarker = regexp.MustCompile(`【\d+:\d+†[^】]+】`)

// NewOverseer creates a new Overseer. mcpApiKey is sent to OpenAI so it can call our MCP
// endpoint for tools.
func NewOverseer(conf *config.Config, logger *slog.Logger, mcpApiKey string) *Overseer {
	client := openai.NewClient(conf.OpenAI.ApiKey)
	return &Overseer{
		client:       client,
		apiKey:       conf.OpenAI.ApiKey,
		mcpKey:       mcpApiKey,
		mcpURL:       conf.OpenAI.McpURL,
		savePath:     conf.SavePath,
		log:          logger.With(sl.Module("overseer")),
		recentOrders: make(map[string]recentOrder),
	}
}

func (o *Overseer) SetRepository(repo Repository) {
	o.repo = repo
}

// SetProductService sets the product service for the Overseer.
// This service is used for product-related operations.
//
// Parameters:
//   - productService: The product service implementation to use
func (o *Overseer) SetProductService(productService ProductService) {
	o.productService = productService
}

// SetAuthService sets the authentication service for the Overseer.
// This service is used for user authentication and basket operations.
//
// Parameters:
//   - authService: The authentication service implementation to use
func (o *Overseer) SetAuthService(authService AuthService) {
	o.authService = authService
}

// SetZohoService sets the Zoho CRM service for the Overseer.
// This service is used for order management in Zoho CRM.
//
// Parameters:
//   - zohoService: The Zoho service implementation to use
func (o *Overseer) SetZohoService(zohoService ZohoService) {
	o.zohoService = zohoService
}

// ComposeResponse generates a response to a user message by determining the appropriate
// assistant to handle the request and routing the message to that assistant.
//
// The method first determines which assistant should handle the request based on the message content,
// then forwards the message to the selected assistant, and finally processes the response.
//
// Parameters:
//   - user: The user entity sending the message
//   - systemMsg: System message providing context
//   - userMsg: The actual message from the user
//
// Returns:
//   - entity.AiAnswer: The AI's response, including text, assistant name, and any product information
//   - error: Any error encountered during processing
func (o *Overseer) ComposeResponse(user *entity.User, systemMsg, userMsg string) (entity.AiAnswer, error) {
	userMsg = truncateRunes(userMsg, maxUserMsgRunes)

	// Initialize empty answer
	answer := entity.AiAnswer{
		Text:      "",
		Assistant: "",
		Products:  nil,
	}

	// Determine which assistant should handle this request
	assistantName, err := o.determineAssistant(user, systemMsg, userMsg)
	if err != nil {
		o.log.With(
			slog.String("userUUID", user.UUID),
			slog.String("system_msg", systemMsg),
			slog.String("user_msg", userMsg),
		).Error("determining assistant", sl.Err(err))
		return answer, err
	}

	o.log.With(
		slog.String("name", assistantName),
	).Debug("determining assistant")

	answer.Assistant = assistantName

	text := ""

	assistant, err := o.repo.GetAssistant(assistantName)
	if err != nil {
		o.log.With(
			slog.String("assistant", assistantName),
			slog.String("userUUID", user.UUID),
		).Error("get assistant", sl.Err(err))
		return answer, fmt.Errorf("failed to get assistant %s: %v", assistantName, err)
	}

	if assistant == nil {
		o.log.With(
			slog.String("assistant", assistantName),
			slog.String("userUUID", user.UUID),
		).Error("assistant not found")
		return answer, fmt.Errorf("assistant %s not found", assistantName)
	}

	if !assistant.Active {
		answer.Text = "Вибачте, цей асистент наразі не активний. Будь ласка, спробуйте пізніше."
		return answer, nil
	}

	//text, answer.Products, err = o.ask(user, userMsg, assistant.Id)
	text, answer.Products, err = o.getResponse(user, userMsg, *assistant)

	// Clean up the response text by removing citation markers
	answer.Text = citationMarker.ReplaceAllString(text, "")

	o.log.With(
		slog.String("userUUID", user.UUID),
		slog.String("assistant", assistantName),
		slog.String("response", answer.Text),
	).Debug("assistant response")

	return answer, err
}

// Product knowledge refresh.
const (
	productFilePrefix      = "products-"
	consultantStoreName    = "assistant-products-store"
	calculatorStoreName    = "calculator-products-store"
	vectorStoreReadyWait   = 10 * time.Minute
	vectorStorePollEvery   = 5 * time.Second
	vectorStoreListPageMax = 100
)

// AttachNewFile uploads today's product list and points the assistants at fresh vector
// stores (runs nightly). Old stores and old product files are deleted only after every
// assistant was switched successfully: deleting first would leave assistants pointing at
// stores that no longer exist, and every AI answer would fail until the next run.
func (o *Overseer) AttachNewFile() error {
	ctx := context.Background()

	products, err := o.productService.GetAvailableProducts()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(products, "", "  ")
	if err != nil {
		o.log.Error("Failed to marshal products", slog.String("error", err.Error()))
		return err
	}

	fileName := filepath.Join(o.savePath, fmt.Sprintf("%s%s.json", productFilePrefix, time.Now().Format("20060102")))
	if err = os.WriteFile(fileName, data, 0o600); err != nil {
		o.log.Error("Failed to write products file", slog.String("error", err.Error()))
		return err
	}
	defer os.Remove(fileName)

	uploadedFile, err := o.client.CreateFile(ctx, openai.FileRequest{
		FilePath: fileName,
		Purpose:  string(openai.PurposeAssistants),
	})
	if err != nil {
		o.log.Error("File upload failed", slog.String("error", err.Error()))
		return err
	}

	filesList, err := o.client.ListFiles(ctx)
	if err != nil {
		o.log.Error("Failed to list files", slog.String("error", err.Error()))
		return err
	}

	// The consultant store holds every assistants file except older product lists. The
	// name is compared by base name because OpenAI may store the full local path or just
	// the file name.
	var consultantFiles, oldProductFiles []string
	for _, file := range filesList.Files {
		if file.Purpose != string(openai.PurposeAssistants) {
			continue
		}
		if file.ID != uploadedFile.ID && strings.HasPrefix(filepath.Base(file.FileName), productFilePrefix) {
			oldProductFiles = append(oldProductFiles, file.ID)
			continue
		}
		consultantFiles = append(consultantFiles, file.ID)
	}

	consultantStore, err := o.client.CreateVectorStore(ctx, openai.VectorStoreRequest{
		Name:    consultantStoreName,
		FileIDs: consultantFiles,
	})
	if err != nil {
		o.log.Error("create consultant vector store", sl.Err(err))
		return err
	}
	calculatorStore, err := o.client.CreateVectorStore(ctx, openai.VectorStoreRequest{
		Name:    calculatorStoreName,
		FileIDs: []string{uploadedFile.ID},
	})
	if err != nil {
		o.log.Error("create calculator vector store", sl.Err(err))
		return err
	}

	// Switch only to stores whose files are indexed, otherwise file_search finds nothing.
	for _, id := range []string{consultantStore.ID, calculatorStore.ID} {
		if err = o.waitVectorStoreReady(ctx, id); err != nil {
			o.log.Error("vector store not ready; keeping previous stores", slog.String("store_id", id), sl.Err(err))
			return err
		}
	}

	switches := []struct{ assistant, store string }{
		{entity.ConsultantAss, consultantStore.ID},
		{entity.CalculatorAss, calculatorStore.ID},
		{entity.OrderManagerAss, calculatorStore.ID},
	}
	switchFailed := false
	for _, sw := range switches {
		if err = o.repo.SetVectorStore(sw.assistant, sw.store); err != nil {
			switchFailed = true
			o.log.Error("set vector store in DB", slog.String("assistant", sw.assistant), sl.Err(err))
		}
	}
	if switchFailed {
		// Some assistants may still use the old stores; keep them and their files.
		return fmt.Errorf("not all assistants switched to new vector stores; old stores kept")
	}

	o.deleteOldVectorStores(ctx, map[string]string{
		consultantStoreName: consultantStore.ID,
		calculatorStoreName: calculatorStore.ID,
	})
	for _, id := range oldProductFiles {
		if err := o.client.DeleteFile(ctx, id); err != nil {
			o.log.Warn("Failed to delete old product file", slog.String("file_id", id), sl.Err(err))
		}
	}

	return nil
}

// waitVectorStoreReady polls until all files of the store are processed.
func (o *Overseer) waitVectorStoreReady(ctx context.Context, storeID string) error {
	deadline := time.Now().Add(vectorStoreReadyWait)
	for {
		vs, err := o.client.RetrieveVectorStore(ctx, storeID)
		if err != nil {
			return fmt.Errorf("retrieve vector store: %w", err)
		}
		if vs.FileCounts.InProgress == 0 {
			if vs.FileCounts.Failed > 0 {
				o.log.Warn("some files failed to index", slog.String("store_id", storeID),
					slog.Int("failed", vs.FileCounts.Failed))
			}
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("files still indexing after %s", vectorStoreReadyWait)
		}
		time.Sleep(vectorStorePollEvery)
	}
}

// deleteOldVectorStores deletes stores whose name is a key of keep but whose ID is not the
// one to keep, walking every page of the store list.
func (o *Overseer) deleteOldVectorStores(ctx context.Context, keep map[string]string) {
	limit := vectorStoreListPageMax
	var after *string
	for {
		page, err := o.client.ListVectorStores(ctx, openai.Pagination{Limit: &limit, After: after})
		if err != nil {
			o.log.Error("Failed to list vector stores", sl.Err(err))
			return
		}
		for _, vs := range page.VectorStores {
			keepID, managed := keep[vs.Name]
			if !managed || vs.ID == "" || vs.ID == keepID {
				continue
			}
			if _, err := o.client.DeleteVectorStore(ctx, vs.ID); err != nil {
				o.log.Warn("delete old vector store", slog.String("store_id", vs.ID), sl.Err(err))
			}
		}
		if !page.HasMore || page.LastID == nil {
			return
		}
		after = page.LastID
	}
}

// truncateRunes cuts s to at most n runes.
func truncateRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
