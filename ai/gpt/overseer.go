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
	"regexp"
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
	imgPath        string
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
		imgPath:    conf.ImgPath,
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
			AssistantID: assistantID,
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

//
//func (a *Assistant) modifyResponse(response GptResponse) (string, error) {
//	productInfo := a.getProductInfo(response.Codes)
//	jsonData, err := json.Marshal(productInfo)
//	if err != nil {
//		return "", err
//	}
//
//	stringProdInfo := string(jsonData)
//	modifiedResponse := "Modify  response with provided information about product, response must include only products from additional info, dont include articles to final response: " + response.Response + "\n Additional info: " + stringProdInfo
//
//	return modifiedResponse, nil
//}
//
//func (a *Assistant) getProductInfo(articles []string) []entity.Product {
//	if a.productService != nil && len(articles) > 0 {
//		products, err := a.productService.GetProductInfo(articles)
//		if err != nil {
//			a.log.Error("getting product info", sl.Err(err))
//		}
//		return products
//	}
//	return []entity.Product{}
//}
//
//func (a *Assistant) AttachNewFile() error {
//	ctx := context.Background()
//
//	products, err := a.productService.GetAvailableProducts()
//	if err != nil {
//		return err
//	}
//
//	// 1. Serialize products to JSON
//	data, err := json.MarshalIndent(products, "", "  ")
//	if err != nil {
//		a.log.Error("Failed to marshal products", slog.String("error", err.Error()))
//		return err
//	}
//
//	prefix := "products-"
//	fileName := fmt.Sprintf("%s/%s%s.json", a.imgPath, prefix, time.Now().Format("20060102"))
//
//	// 2. Create a temporary file
//	f, err := os.Create(fileName)
//	if err != nil {
//		a.log.Error("Failed to create temporary file", slog.String("error", err.Error()))
//		return err
//	}
//	defer os.Remove(f.Name()) // Ensure cleanup
//
//	// 3. Write serialized data to the temp file
//	if _, err := f.Write(data); err != nil {
//		f.Close()
//		a.log.Error("Failed to write to temporary file", slog.String("error", err.Error()))
//		return err
//	}
//
//	// Close the file after writing
//	if err := f.Close(); err != nil {
//		a.log.Error("Failed to close temporary file", slog.String("error", err.Error()))
//		return err
//	}
//
//	// 4. Upload the file
//	uploadedFile, err := a.client.CreateFile(ctx, openai.FileRequest{
//		FilePath: f.Name(),
//		Purpose:  string(openai.PurposeAssistants),
//	})
//	if err != nil {
//		a.log.Error("File upload failed", slog.String("error", err.Error()))
//		return err
//	}
//
//	// 5. List files and delete previous ones (except the new one)
//	filesList, err := a.client.ListFiles(ctx)
//	if err != nil {
//		a.log.Error("Failed to list files", slog.String("error", err.Error()))
//		return err
//	}
//
//	var fileIds []string
//
//	for _, file := range filesList.Files {
//		if file.Purpose == string(openai.PurposeAssistants) &&
//			file.ID != uploadedFile.ID &&
//			strings.HasPrefix(file.FileName, fmt.Sprintf("%s/%s", a.imgPath, prefix)) {
//			if err := a.client.DeleteFile(ctx, file.ID); err != nil {
//				a.log.Warn("Failed to delete file", slog.String("file_id", file.ID), slog.String("error", err.Error()))
//			}
//		} else if file.Purpose == string(openai.PurposeAssistants) {
//			fileIds = append(fileIds, file.ID)
//		}
//	}
//
//	// 6. Create a new vector store with the file
//	newStore, err := a.client.CreateVectorStore(ctx, openai.VectorStoreRequest{
//		Name:    "assistant-products-store",
//		FileIDs: fileIds,
//	})
//	if err != nil {
//		a.log.Error("Failed to create new vector store", slog.String("error", err.Error()))
//		return err
//	}
//
//	// 7. Attach new vector store to assistant
//	_, err = a.client.ModifyAssistant(ctx, a.assistantID, openai.AssistantRequest{
//		ToolResources: &openai.AssistantToolResource{
//			FileSearch: &openai.AssistantToolFileSearch{
//				VectorStoreIDs: []string{newStore.ID},
//			},
//		},
//	})
//	if err != nil {
//		a.log.Error("Failed to attach new vector store", slog.String("error", err.Error()))
//		return err
//	}
//
//	// 8. Delete previous vector store (if known and different)
//	storesList, err := a.client.ListVectorStores(ctx, openai.Pagination{})
//	if err != nil {
//		a.log.Error("Failed to list files", slog.String("error", err.Error()))
//		return err
//	}
//	for _, vs := range storesList.VectorStores {
//		if vs.ID != "" && vs.ID != newStore.ID && vs.Name == newStore.Name {
//			_, err := a.client.DeleteVectorStore(ctx, vs.ID)
//			if err != nil {
//				a.log.Warn("Failed to delete old vector store", slog.String("error", err.Error()))
//			}
//		}
//	}
//
//	return nil
//}
//
//func (a *Assistant) GenerateImg(user entity.User, userPrompt string) (string, error) {
//	ctx := context.Background()
//	var thread openai.Thread
//	var err error
//
//	chatId := user.ChatId
//
//	threadId := a.threads[chatId]
//	if threadId != "" {
//		thread, err = a.client.RetrieveThread(context.Background(), threadId)
//		if err != nil {
//			a.log.With(slog.String("thread", threadId)).Error("retrieving thread", sl.Err(err))
//		}
//	} else {
//		err = fmt.Errorf("threadId is empty")
//	}
//
//	if err != nil {
//		thread, err = a.client.CreateThread(context.Background(), openai.ThreadRequest{})
//		if err != nil {
//			return "", err
//		}
//		a.threads[chatId] = thread.ID
//		a.log.With(slog.String("thread", thread.ID)).Info("created new thread")
//	}
//
//	// 2. Send user prompt as message
//	_, err = a.client.CreateMessage(ctx, thread.ID, openai.MessageRequest{
//		Role:    string(openai.ThreadMessageRoleUser),
//		Content: userPrompt,
//	})
//	if err != nil {
//		return "", fmt.Errorf("failed to create message: %w", err)
//	}
//
//	// 3. Create run
//	completed := a.handleRun(thread.ID, user.AssistantId)
//	if !completed {
//		return "", fmt.Errorf("max retries reached, unable to complete run")
//	}
//
//	// 5. Get assistant messages and parse function response
//	msgs, err := a.client.ListMessage(ctx, thread.ID, nil, nil, nil, nil, nil)
//	if err != nil {
//		return "", fmt.Errorf("failed to list messages: %w", err)
//	}
//
//	responseText := msgs.Messages[0].Content[0].Text.Value
//
//	var imgResp *entity.ImgGenResponse
//	err = json.Unmarshal([]byte(responseText), &imgResp)
//	if err != nil {
//		return "", err
//	}
//	//imgResp.Text = userPrompt
//	imgResp.ChatId = user.ChatId
//
//	a.log.With(
//		slog.String("type", imgResp.Type),
//		slog.String("prompt", imgResp.Text),
//	).Debug("response")
//
//	return a.CallImageAPI(ctx, *imgResp)
//}
//
//func (a *Assistant) CallImageAPI(ctx context.Context, imgResp entity.ImgGenResponse) (string, error) {
//	imgPath := fmt.Sprintf("%s/%d.png", a.imgPath, imgResp.ChatId)
//
//	switch imgResp.Type {
//	case entity.CreateType:
//		resp, err := a.client.CreateImage(ctx, openai.ImageRequest{
//			Model:   openai.CreateImageModelGptImage1,
//			Prompt:  imgResp.Text,
//			N:       1,
//			Size:    "1024x1024",
//			Quality: openai.CreateImageQualityMedium,
//		})
//		if err != nil {
//			return "", fmt.Errorf("image generation failed: %w", err)
//		}
//		if len(resp.Data) == 0 {
//			return "", fmt.Errorf("no image returned")
//		}
//
//		err = saveBase64Image(resp.Data[0].B64JSON, imgPath)
//		if err != nil {
//			return "", fmt.Errorf("failed to save image: %w", err)
//		}
//		return imgPath, nil
//
//	case entity.EditType:
//		imageFile, err := os.Open(imgPath)
//		if err != nil {
//			return "", fmt.Errorf("cannot open image: %w", err)
//		}
//		defer imageFile.Close()
//
//		b64Image, err := a.sendImageEditRequest(imageFile, filepath.Base(imgPath), imgResp.Text, "1024x1024", openai.CreateImageQualityMedium, 1)
//		if err != nil {
//			return "", fmt.Errorf("image edit failed: %w", err)
//		}
//
//		err = saveBase64Image(b64Image, imgPath)
//		if err != nil {
//			return "", fmt.Errorf("failed to save image: %w", err)
//		}
//
//		return imgPath, nil
//
//	case entity.VariantType:
//		imageFile, err := os.Open(imgPath)
//		if err != nil {
//			return "", fmt.Errorf("base image not found for variant: %w", err)
//		}
//		defer imageFile.Close()
//
//		resp, err := a.client.CreateVariImage(ctx, openai.ImageVariRequest{
//			Model:          openai.CreateImageModelGptImage1,
//			Image:          imageFile,
//			N:              1,
//			Size:           "1024x1024",
//			ResponseFormat: openai.CreateImageResponseFormatB64JSON,
//		})
//		if err != nil {
//			return "", fmt.Errorf("image variation failed: %w", err)
//		}
//		if len(resp.Data) == 0 {
//			return "", fmt.Errorf("no image returned on variant")
//		}
//
//		err = saveBase64Image(resp.Data[0].B64JSON, imgPath)
//		if err != nil {
//			return "", fmt.Errorf("failed to save image: %w", err)
//		}
//
//		return imgPath, nil
//
//	default:
//		return "", fmt.Errorf("unknown image operation type: %s", imgResp.Type)
//	}
//}
//
//func (a *Assistant) sendImageEditRequest(imageFile *os.File, filename, prompt, size, quality string, n int) (string, error) {
//	var requestBody bytes.Buffer
//	writer := multipart.NewWriter(&requestBody)
//
//	_ = writer.WriteField("model", "gpt-image-1")
//	_ = writer.WriteField("prompt", prompt)
//	_ = writer.WriteField("size", size)
//	_ = writer.WriteField("quality", quality)
//	_ = writer.WriteField("n", fmt.Sprintf("%d", n))
//
//	// Add image
//	imageHeader := make(textproto.MIMEHeader)
//	imageHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="%s"`, filename))
//	imageHeader.Set("Content-Type", "image/png")
//
//	imagePart, err := writer.CreatePart(imageHeader)
//	if err != nil {
//		return "", fmt.Errorf("failed to create image part: %w", err)
//	}
//	if _, err := io.Copy(imagePart, imageFile); err != nil {
//		return "", fmt.Errorf("failed to copy image data: %w", err)
//	}
//
//	_ = writer.Close()
//
//	req, err := http.NewRequest("POST", "https://api.openai.com/v1/images/edits", &requestBody)
//	if err != nil {
//		return "", fmt.Errorf("failed to create request: %w", err)
//	}
//	req.Header.Set("Authorization", "Bearer "+a.apiKey)
//	req.Header.Set("Content-Type", writer.FormDataContentType())
//
//	resp, err := http.DefaultClient.Do(req)
//	if err != nil {
//		return "", fmt.Errorf("request failed: %w", err)
//	}
//	defer resp.Body.Close()
//
//	if resp.StatusCode != http.StatusOK {
//		body, _ := io.ReadAll(resp.Body)
//		return "", fmt.Errorf("status %d, body: %s", resp.StatusCode, string(body))
//	}
//
//	var result struct {
//		Data []struct {
//			B64JSON string `json:"b64_json"`
//		} `json:"data"`
//	}
//	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
//		return "", fmt.Errorf("failed to decode response: %w", err)
//	}
//	if len(result.Data) == 0 {
//		return "", fmt.Errorf("no image returned")
//	}
//
//	return result.Data[0].B64JSON, nil
//}
//
//func (a *Assistant) SaveUrlImage(imageURL, fileName string) error {
//	imgPath := fmt.Sprintf("%s/%s.png", a.imgPath, fileName)
//
//	// Fetch the image from the URL
//	resp, err := http.Get(imageURL)
//	if err != nil {
//		return fmt.Errorf("failed to download image: %w", err)
//	}
//	defer resp.Body.Close()
//
//	if resp.StatusCode != http.StatusOK {
//		return fmt.Errorf("failed to download image, status code: %d", resp.StatusCode)
//	}
//
//	// Decode the image (detects format)
//	img, _, err := image.Decode(resp.Body)
//	if err != nil {
//		return fmt.Errorf("failed to decode image: %w", err)
//	}
//
//	// Create output file
//	outFile, err := os.Create(imgPath)
//	if err != nil {
//		return fmt.Errorf("failed to create output file: %w", err)
//	}
//	defer outFile.Close()
//
//	// Encode and save as PNG
//	if err := png.Encode(outFile, img); err != nil {
//		return fmt.Errorf("failed to encode PNG: %w", err)
//	}
//
//	return nil
//}
//
//func saveBase64Image(b64data, filepath string) error {
//	data, err := base64.StdEncoding.DecodeString(b64data)
//	if err != nil {
//		return fmt.Errorf("failed to decode base64 image: %w", err)
//	}
//
//	err = os.WriteFile(filepath, data, 0644)
//	if err != nil {
//		return fmt.Errorf("failed to write image file: %w", err)
//	}
//	return nil
//}
