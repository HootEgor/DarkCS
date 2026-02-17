// Package gpt provides functionality for handling AI-powered interactions and commands
// in the DarkCS system. This package includes AI assistant management, thread handling,
// and integration with various services for product, authentication, and order management.
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
	// UpdateUser updates a user's information in the system
	UpdateUser(user *entity.User) error

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
// It handles OpenAI API communication, thread management, and service integration.
type Overseer struct {
	client         *openai.Client        // OpenAI API client
	assistants     map[string]string     // Map of assistant names to their IDs
	apiKey         string                // OpenAI API key
	mcpKey         string                // MCP API key
	threads        map[string]ThreadMeta // Map of user IDs to their thread metadata
	productService ProductService        // Service for product-related operations
	authService    AuthService           // Service for authentication and user operations
	zohoService    ZohoService           // Service for Zoho CRM integration
	repo           Repository
	savePath       string       // Path for saving files
	locker         *LockThreads // Thread locking mechanism
	log            *slog.Logger // Logger instance
}

// ThreadMeta stores metadata about a conversation thread.
type ThreadMeta struct {
	ID           string // OpenAI thread ID
	MessageCount int    // Number of messages in the thread
}

// LockThreads provides thread-safe access to user threads.
// It uses a mutex to synchronize access to the threads map.
type LockThreads struct {
	mutex   sync.Mutex             // Mutex for synchronizing access to the threads map
	threads map[string]*sync.Mutex // Map of user IDs to their thread mutexes
}

// OverseerResponse represents the response structure from the Overseer.
type OverseerResponse struct {
	Assistant string `json:"assistant"` // Name of the assistant that handled the request
}

// NewOverseer creates a new instance of the Overseer with the provided configuration and logger.
// It initializes the OpenAI client, sets up assistant mappings, and prepares thread management.
//
// Parameters:
//   - conf: Configuration containing OpenAI API keys and assistant IDs
//   - logger: Logger instance for recording operations
//
// Returns:
//   - *Overseer: A new Overseer instance ready for use
func NewOverseer(conf *config.Config, logger *slog.Logger, mcpApiKey string) *Overseer {
	client := openai.NewClient(conf.OpenAI.ApiKey)
	assistants := make(map[string]string)
	assistants[entity.OverseerAss] = conf.OpenAI.OverseerID
	assistants[entity.ConsultantAss] = conf.OpenAI.ConsultantID
	assistants[entity.CalculatorAss] = conf.OpenAI.CalculatorID
	assistants[entity.OrderManagerAss] = conf.OpenAI.OrderManagerID
	return &Overseer{
		client:     client,
		assistants: assistants,
		apiKey:     conf.OpenAI.ApiKey,
		mcpKey:     mcpApiKey,
		threads:    make(map[string]ThreadMeta),
		savePath:   conf.SavePath,
		locker:     &LockThreads{threads: make(map[string]*sync.Mutex)},
		log:        logger.With(sl.Module("overseer")),
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

// Lock acquires a lock for the specified user's thread.
// This ensures thread-safe access to user-specific resources.
// If a mutex doesn't exist for the user, it creates one.
//
// Parameters:
//   - userId: The ID of the user whose thread should be locked
func (l *LockThreads) Lock(userId string) {
	l.mutex.Lock()

	mutex, exists := l.threads[userId]
	if !exists {
		mutex = &sync.Mutex{}
		l.threads[userId] = mutex
	}

	l.mutex.Unlock()

	mutex.Lock()
}

// Unlock releases the lock for the specified user's thread.
// If no lock exists for the user, it does nothing.
//
// Parameters:
//   - userId: The ID of the user whose thread should be unlocked
func (l *LockThreads) Unlock(userId string) {
	l.mutex.Lock()

	mutex, exists := l.threads[userId]
	if !exists {
		l.mutex.Unlock()
		return
	}
	l.mutex.Unlock()

	mutex.Unlock()
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
	re := regexp.MustCompile(`【\d+:\d+†[^】]+】`)
	answer.Text = re.ReplaceAllString(text, "")

	o.log.With(
		slog.String("userUUID", user.UUID),
		slog.String("assistant", assistantName),
		slog.String("response", answer.Text),
	).Debug("assistant response")

	return answer, err
}

// determineAssistant analyzes a user message to decide which specialized assistant
// should handle the request. It uses the Overseer assistant to make this determination.
//
// The method creates or retrieves a conversation thread for the user, sends the message
// to the Overseer assistant, and interprets the response to determine which specialized
// assistant (consultant, calculator, order manager) should handle the actual request.
//
// Parameters:
//   - user: The user entity sending the message
//   - systemMsg: System message providing context
//   - userMsg: The actual message from the user
//
// Returns:
//   - string: The name of the assistant that should handle the request
//   - error: Any error encountered during processing
//func (o *Overseer) determineAssistant(user *entity.User, systemMsg, userMsg string) (string, error) {
//	// Ensure the thread is unlocked when this function completes
//	defer o.locker.Unlock(user.UUID)
//
//	// Get or create a conversation thread for this user
//	thread, err := o.getOrCreateThread(user.UUID)
//	if err != nil {
//		return "", err
//	}
//
//	// Combine system message and user message
//	question := fmt.Sprintf("%s, HttpUserMsg: %s", systemMsg, userMsg)
//
//	// Send the user message to the assistant
//	_, err = o.client.CreateMessage(context.Background(), thread.ID, openai.MessageRequest{
//		Role:    string(openai.ThreadMessageRoleUser),
//		Content: question,
//	})
//	if err != nil {
//		return "", fmt.Errorf("error creating message: %v", err)
//	}
//
//	// Run the Overseer assistant to analyze the message
//	completed := o.handleRun(user, thread.ID, o.assistants[entity.OverseerAss])
//	if !completed {
//		return "", fmt.Errorf("max retries reached, unable to complete run")
//	}
//
//	// Fetch the messages once the run is complete
//	msgs, err := o.client.ListMessage(context.Background(), thread.ID, nil, nil, nil, nil, nil)
//	if err != nil {
//		return "", fmt.Errorf("error listing messages: %v", err)
//	}
//
//	if len(msgs.Messages) == 0 {
//		return "", fmt.Errorf("no messages found")
//	}
//
//	// Extract the response text from the assistant
//	responseText := msgs.Messages[0].Content[0].Text.Value
//
//	// Parse the response to determine which assistant should handle the request
//	var response OverseerResponse
//	err = json.Unmarshal([]byte(responseText), &response)
//	if err != nil {
//		o.log.With(
//			slog.String("userUUID", user.UUID),
//			slog.Int("text_length", len(responseText)),
//			slog.String("response", responseText),
//		).Debug("chat response")
//		o.log.Error("error unmarshalling response", sl.Err(err))
//		return responseText, nil
//	}
//
//	return response.Assistant, nil
//}

func (o *Overseer) handleRun(user *entity.User, threadID, assistantID string) bool {
	maxRetries := 3
	completed := false
	ctx := context.Background()

	for attempt := 0; attempt < maxRetries; attempt++ {
		run, err := o.client.CreateRun(ctx, threadID, openai.RunRequest{
			AssistantID:       assistantID,
			ParallelToolCalls: false,
		})
		if err != nil {
			o.log.Error(fmt.Sprintf("error creating run: %v", err))
			continue
		}

		nextPoll := false
		for {
			time.Sleep(1 * time.Second)
			run, err = o.client.RetrieveRun(ctx, threadID, run.ID)
			if err != nil {
				o.log.Error(fmt.Sprintf("error retrieving run: %v", err))
				break
			}

			switch run.Status {
			case openai.RunStatusCompleted:
				completed = true
				nextPoll = true
				break
			case openai.RunStatusRequiresAction:
				if run.RequiredAction.Type == openai.RequiredActionTypeSubmitToolOutputs {
					var toolOutputs []openai.ToolOutput

					for _, toolCall := range run.RequiredAction.SubmitToolOutputs.ToolCalls {
						cmdName := toolCall.Function.Name
						cmdArgs := toolCall.Function.Arguments
						output, err := o.HandleCommand(user, cmdName, []byte(cmdArgs))
						if err != nil {
							o.log.With(
								slog.String("command", cmdName),
								slog.Any("args", cmdArgs),
								sl.Err(err),
							).Error("handling command")
							output = fmt.Sprintf("Error handling command %s: %v", cmdName, err)
						}

						toolOutputs = append(toolOutputs, openai.ToolOutput{
							ToolCallID: toolCall.ID,
							Output:     fmt.Sprintf("%s", output),
						})
					}

					run, err = o.client.SubmitToolOutputs(ctx, threadID, run.ID, openai.SubmitToolOutputsRequest{
						ToolOutputs: toolOutputs,
					})
					if err != nil {
						o.log.With(
							sl.Err(err),
						).Error("submitting tool outputs")
					}
				}
				break
			case openai.RunStatusFailed, openai.RunStatusCancelled, openai.RunStatusExpired, openai.RunStatusIncomplete:
				errorMsg := ""
				if run.LastError != nil {
					errorMsg = run.LastError.Message
				}
				o.log.With(
					slog.String("status", string(run.Status)),
					slog.Any("error", errorMsg),
				).Error(fmt.Sprintf("run failed"))
				nextPoll = true
				break
			default:
				// still running, continue polling
			}

			if nextPoll {
				break
			}
		}

		if completed {
			break
		}

		time.Sleep(2 * time.Second)
	}

	return completed
}

func (o *Overseer) getOrCreateThread(userId string) (openai.Thread, error) {
	o.locker.Lock(userId)

	meta, exists := o.threads[userId]
	shouldReset := false

	if exists {
		if meta.MessageCount > 20 {
			shouldReset = true
		}
	}

	// Reset thread if needed
	if shouldReset {
		o.log.With(slog.String("thread", meta.ID)).Info("resetting thread, creating summary")

		// 1. Fetch all messages
		msgs, err := o.client.ListMessage(context.Background(), meta.ID, nil, nil, nil, nil, nil)
		if err != nil {
			o.log.Warn("failed to fetch old messages for summary", sl.Err(err))
		} else {
			// 2. Generate summary
			summary := o.summarizeMessages(msgs.Messages)

			// 3. Create a new thread
			thread, err := o.client.CreateThread(context.Background(), openai.ThreadRequest{})
			if err != nil {
				return openai.Thread{}, err
			}

			// 4. Add summary as first assistant message
			_, err = o.client.CreateMessage(context.Background(), thread.ID, openai.MessageRequest{
				Role:    string(openai.ThreadMessageRoleAssistant),
				Content: fmt.Sprintf("Ось короткий підсумок нашої попередньої розмови, щоб ми могли продовжити:\n\n%s", summary),
			})
			if err != nil {
				o.log.Warn("failed to add summary to new thread", sl.Err(err))
			}

			// 5. Replace thread
			o.threads[userId] = ThreadMeta{
				ID:           thread.ID,
				MessageCount: 0,
			}
			return thread, nil
		}
	}

	// Reuse an existing thread if it exists
	if threadMeta, ok := o.threads[userId]; ok && threadMeta.ID != "" {
		thread, err := o.client.RetrieveThread(context.Background(), threadMeta.ID)
		if err == nil {
			meta.MessageCount++
			o.threads[userId] = meta
			return thread, nil
		}
	}

	// Create new thread
	thread, err := o.client.CreateThread(context.Background(), openai.ThreadRequest{})
	if err != nil {
		return openai.Thread{}, err
	}

	o.threads[userId] = ThreadMeta{
		ID:           thread.ID,
		MessageCount: 0,
	}

	return thread, nil
}

func (o *Overseer) summarizeMessages(msgs []openai.Message) string {
	ctx := context.Background()

	var history []string
	for _, msg := range msgs {
		for _, content := range msg.Content {
			history = append(history, fmt.Sprintf("[%s]: %s", msg.Role, content.Text.Value))
		}
	}

	historyText := strings.Join(history, "\n")

	summaryPrompt := fmt.Sprintf(`Будь ласка, коротко підсумуй діалог нижче українською мовою. Залиш тільки основні моменти, без деталей замовлення чи товарних кодів. %s`, historyText)

	resp, err := o.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT4oMini, // or "gpt-4o"
		Messages: []openai.ChatCompletionMessage{
			{Role: "system", Content: "Ти — помічниця, яка коротко підсумовує розмови користувачів."},
			{Role: "user", Content: summaryPrompt},
		},
		MaxTokens:   300,
		Temperature: 0.3,
	})
	if err != nil || len(resp.Choices) == 0 {
		o.log.Warn("failed to generate summary", sl.Err(err))
		return "На жаль, не вдалося створити підсумок попередньої розмови."
	}

	return resp.Choices[0].Message.Content
}

func (o *Overseer) AttachNewFile() error {
	ctx := context.Background()

	products, err := o.productService.GetAvailableProducts()
	if err != nil {
		return err
	}

	// 1. Serialize products to JSON
	data, err := json.MarshalIndent(products, "", "  ")
	if err != nil {
		o.log.Error("Failed to marshal products", slog.String("error", err.Error()))
		return err
	}

	prefix := "products-"
	fileName := fmt.Sprintf("%s/%s%s.json", o.savePath, prefix, time.Now().Format("20060102"))

	// 2. Create a temporary file
	f, err := os.Create(fileName)
	if err != nil {
		o.log.Error("Failed to create temporary file", slog.String("error", err.Error()))
		return err
	}
	defer os.Remove(f.Name()) // Ensure cleanup

	// 3. Write serialized data to the temp file
	if _, err := f.Write(data); err != nil {
		f.Close()
		o.log.Error("Failed to write to temporary file", slog.String("error", err.Error()))
		return err
	}

	// Close the file after writing
	if err := f.Close(); err != nil {
		o.log.Error("Failed to close temporary file", slog.String("error", err.Error()))
		return err
	}

	// 4. Upload the file
	uploadedFile, err := o.client.CreateFile(ctx, openai.FileRequest{
		FilePath: f.Name(),
		Purpose:  string(openai.PurposeAssistants),
	})
	if err != nil {
		o.log.Error("File upload failed", slog.String("error", err.Error()))
		return err
	}

	// 5. List files and delete previous ones (except the new one)
	filesList, err := o.client.ListFiles(ctx)
	if err != nil {
		o.log.Error("Failed to list files", slog.String("error", err.Error()))
		return err
	}

	var fileIds []string

	for _, file := range filesList.Files {
		if file.Purpose == string(openai.PurposeAssistants) &&
			file.ID != uploadedFile.ID &&
			strings.HasPrefix(file.FileName, fmt.Sprintf("%s/%s", o.savePath, prefix)) {
			if err := o.client.DeleteFile(ctx, file.ID); err != nil {
				o.log.Warn("Failed to delete file", slog.String("file_id", file.ID), slog.String("error", err.Error()))
			}
		} else if file.Purpose == string(openai.PurposeAssistants) {
			fileIds = append(fileIds, file.ID)
		}
	}

	// 6. Create a new vector store with the file
	consultantStore, err := o.client.CreateVectorStore(ctx, openai.VectorStoreRequest{
		Name:    "assistant-products-store",
		FileIDs: fileIds,
	})
	if err != nil {
		o.log.With(
			sl.Err(err),
		).Error("create consultant vector store")
		return err
	}

	calculatorStore, err := o.client.CreateVectorStore(ctx, openai.VectorStoreRequest{
		Name:    "calculator-products-store",
		FileIDs: []string{uploadedFile.ID},
	})
	if err != nil {
		o.log.With(
			sl.Err(err),
		).Error("create calculator vector store")
		return err
	}

	// 7. Attach new vector store to assistant
	//_, err = o.client.ModifyAssistant(ctx, o.assistants[entity.ConsultantAss], openai.AssistantRequest{
	//	ToolResources: &openai.AssistantToolResource{
	//		FileSearch: &openai.AssistantToolFileSearch{
	//			VectorStoreIDs: []string{consultantStore.ID},
	//		},
	//	},
	//})
	//if err != nil {
	//	o.log.With(
	//		sl.Err(err),
	//	).Error("attach consultant vector store")
	//	return err
	//}
	err = o.repo.SetVectorStore(entity.ConsultantAss, consultantStore.ID)
	if err != nil {
		o.log.With(
			sl.Err(err),
		).Error("set vector store in DB")
	}

	// Attach calculator store to calculator assistant
	//_, err = o.client.ModifyAssistant(ctx, o.assistants[entity.CalculatorAss], openai.AssistantRequest{
	//	ToolResources: &openai.AssistantToolResource{
	//		FileSearch: &openai.AssistantToolFileSearch{
	//			VectorStoreIDs: []string{calculatorStore.ID},
	//		},
	//	},
	//})
	//if err != nil {
	//	o.log.With(
	//		sl.Err(err),
	//	).Error("attach calculator vector store")
	//	return err
	//}
	err = o.repo.SetVectorStore(entity.CalculatorAss, calculatorStore.ID)
	if err != nil {
		o.log.With(
			sl.Err(err),
		).Error("set calculator vector store in DB")
	}

	// Attach calculator store to order manager assistant
	//_, err = o.client.ModifyAssistant(ctx, o.assistants[entity.OrderManagerAss], openai.AssistantRequest{
	//	ToolResources: &openai.AssistantToolResource{
	//		FileSearch: &openai.AssistantToolFileSearch{
	//			VectorStoreIDs: []string{calculatorStore.ID},
	//		},
	//	},
	//})
	//if err != nil {
	//	o.log.With(
	//		sl.Err(err),
	//	).Error("attach order manager vector store")
	//	return err
	//}
	err = o.repo.SetVectorStore(entity.OrderManagerAss, calculatorStore.ID)
	if err != nil {
		o.log.With(
			sl.Err(err),
		).Error("set order manager vector store in DB")
	}

	// 8. Delete previous vector stores
	storesList, err := o.client.ListVectorStores(ctx, openai.Pagination{})
	if err != nil {
		o.log.Error("Failed to list files", slog.String("error", err.Error()))
		return err
	}
	for _, vs := range storesList.VectorStores {
		if vs.ID != "" && vs.ID != consultantStore.ID && vs.Name == consultantStore.Name {
			_, err = o.client.DeleteVectorStore(ctx, vs.ID)
			if err != nil {
				o.log.With(
					sl.Err(err),
				).Warn("delete old consultant vector store")
			}
		}

		if vs.ID != "" && vs.ID != calculatorStore.ID && vs.Name == calculatorStore.Name {
			_, err = o.client.DeleteVectorStore(ctx, vs.ID)
			if err != nil {
				o.log.With(
					sl.Err(err),
				).Warn("delete old calculator vector store")
			}
		}
	}

	return nil
}
