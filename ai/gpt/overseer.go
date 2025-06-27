package gpt

import (
	"DarkCS/entity"
	"DarkCS/internal/config"
	"DarkCS/internal/lib/sl"
	"context"
	"encoding/json"
	"fmt"
	"github.com/sashabaranov/go-openai"
	_ "image/jpeg"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type ProductService interface {
	GetProductInfo(articles []string) ([]entity.ProductInfo, error)
	GetAvailableProducts() ([]entity.Product, error)
}

type AuthService interface {
	UpdateUserPhone(email, phone string, telegramId int64) error

	GetBasket(userUUID string) (*entity.Basket, error)
	AddToBasket(userUUID string, products []entity.OrderProduct) (*entity.Basket, error)
	RemoveFromBasket(userUUID string, products []entity.OrderProduct) (*entity.Basket, error)
}

type Overseer struct {
	client         *openai.Client
	assistants     map[string]string
	apiKey         string
	threads        map[string]string
	productService ProductService
	authService    AuthService
	savePath       string
	locker         *LockThreads
	log            *slog.Logger
}

type LockThreads struct {
	mutex   sync.Mutex
	threads map[string]*sync.Mutex
}

type OverseerResponse struct {
	Assistant string `json:"assistant"`
}

func NewOverseer(conf *config.Config, logger *slog.Logger) *Overseer {
	client := openai.NewClient(conf.OpenAI.ApiKey)
	assistants := make(map[string]string)
	assistants[entity.OverseerAss] = conf.OpenAI.OverseerID
	assistants[entity.ConsultantAss] = conf.OpenAI.ConsultantID
	assistants[entity.CalculatorAss] = conf.OpenAI.CalculatorID
	return &Overseer{
		client:     client,
		assistants: assistants,
		apiKey:     conf.OpenAI.ApiKey,
		threads:    make(map[string]string),
		savePath:   conf.SavePath,
		locker:     &LockThreads{threads: make(map[string]*sync.Mutex)},
		log:        logger.With(sl.Module("overseer")),
	}
}

func (o *Overseer) SetProductService(productService ProductService) {
	o.productService = productService
}

func (o *Overseer) SetAuthService(authService AuthService) {
	o.authService = authService
}

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

func (o *Overseer) ComposeResponse(user *entity.User, systemMsg, userMsg string) (entity.AiAnswer, error) {
	answer := entity.AiAnswer{
		Text:      "",
		Assistant: "",
		Products:  nil,
	}

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

	switch assistantName {
	case entity.ConsultantAss:
		text, answer.Products, err = o.askConsultant(user, userMsg)
		break
	case entity.CalculatorAss:
		text, answer.Products, err = o.askCalculator(user, userMsg)
		break
	default:
		text, answer.Products, err = o.askConsultant(user, userMsg)
	}

	re := regexp.MustCompile(`【\d+:\d+†[^】]+】`)
	answer.Text = re.ReplaceAllString(text, "")

	return answer, err
}

func (o *Overseer) determineAssistant(user *entity.User, systemMsg, userMsg string) (string, error) {
	defer o.locker.Unlock(user.UUID)
	thread, err := o.getOrCreateThread(user.UUID)
	if err != nil {
		return "", err
	}

	question := fmt.Sprintf("%s, HttpUserMsg: %s", systemMsg, userMsg)
	// Send the user message to the assistant
	_, err = o.client.CreateMessage(context.Background(), thread.ID, openai.MessageRequest{
		Role:    string(openai.ThreadMessageRoleUser),
		Content: question,
	})
	if err != nil {
		return "", fmt.Errorf("error creating message: %v", err)
	}

	completed := o.handleRun(user, thread.ID, o.assistants[entity.OverseerAss])
	if !completed {
		return "", fmt.Errorf("max retries reached, unable to complete run")
	}

	// Fetch the messages once the run is complete
	msgs, err := o.client.ListMessage(context.Background(), thread.ID, nil, nil, nil, nil, nil)
	if err != nil {
		return "", fmt.Errorf("error listing messages: %v", err)
	}

	if len(msgs.Messages) == 0 {
		return "", fmt.Errorf("no messages found")
	}

	responseText := msgs.Messages[0].Content[0].Text.Value

	var response OverseerResponse
	err = json.Unmarshal([]byte(responseText), &response)
	if err != nil {
		o.log.With(
			slog.String("userUUID", user.UUID),
			slog.Int("text_length", len(responseText)),
			slog.String("response", responseText),
		).Debug("chat response")
		o.log.Error("error unmarshalling response", sl.Err(err))
		return responseText, nil
	}

	return response.Assistant, nil
}

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
						output, err := o.handleCommand(user, cmdName, cmdArgs)
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

	if threadId, ok := o.threads[userId]; ok && threadId != "" {
		thread, err := o.client.RetrieveThread(context.Background(), threadId)
		if err == nil {
			return thread, nil
		}
		o.log.With(slog.String("thread", threadId)).Error("retrieving thread", sl.Err(err))
	}

	thread, err := o.client.CreateThread(context.Background(), openai.ThreadRequest{})
	if err != nil {
		return openai.Thread{}, err
	}

	o.threads[userId] = thread.ID
	o.log.With(slog.String("thread", thread.ID)).Info("created new thread")

	return thread, nil
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
	_, err = o.client.ModifyAssistant(ctx, o.assistants[entity.ConsultantAss], openai.AssistantRequest{
		ToolResources: &openai.AssistantToolResource{
			FileSearch: &openai.AssistantToolFileSearch{
				VectorStoreIDs: []string{calculatorStore.ID},
			},
		},
	})
	if err != nil {
		o.log.With(
			sl.Err(err),
		).Error("attach consultant vector store")
		return err
	}

	// Attach calculator store to calculator assistant
	_, err = o.client.ModifyAssistant(ctx, o.assistants[entity.CalculatorAss], openai.AssistantRequest{
		ToolResources: &openai.AssistantToolResource{
			FileSearch: &openai.AssistantToolFileSearch{
				VectorStoreIDs: []string{calculatorStore.ID},
			},
		},
	})
	if err != nil {
		o.log.With(
			sl.Err(err),
		).Error("attach calculator vector store")
		return err
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
