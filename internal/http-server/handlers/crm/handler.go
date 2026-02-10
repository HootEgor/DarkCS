package crm

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"DarkCS/entity"
	"DarkCS/internal/lib/api/response"
)

// Core defines the methods required by CRM handlers.
type Core interface {
	GetActiveChats() ([]entity.ChatSummary, error)
	GetChatMessages(platform, userID string, limit, offset int) ([]entity.ChatMessage, error)
	SendCrmMessage(platform, userID, text string) error
}

// GetChats returns the list of active chats with last message info.
func GetChats(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chats, err := handler.GetActiveChats()
		if err != nil {
			log.Error("failed to get active chats", slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("Failed to get chats"))
			return
		}

		if chats == nil {
			chats = []entity.ChatSummary{}
		}

		render.JSON(w, r, response.Ok(chats))
	}
}

// GetMessages returns paginated message history for a specific chat.
func GetMessages(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		platform := chi.URLParam(r, "platform")
		userID := chi.URLParam(r, "user_id")

		if platform == "" || userID == "" {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("platform and user_id are required"))
			return
		}

		limit := 50
		offset := 0
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
				limit = v
			}
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			if v, err := strconv.Atoi(o); err == nil && v >= 0 {
				offset = v
			}
		}

		messages, err := handler.GetChatMessages(platform, userID, limit, offset)
		if err != nil {
			log.Error("failed to get chat messages",
				slog.String("platform", platform),
				slog.String("user_id", userID),
				slog.String("error", err.Error()),
			)
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("Failed to get messages"))
			return
		}

		if messages == nil {
			messages = []entity.ChatMessage{}
		}

		render.JSON(w, r, response.Ok(messages))
	}
}

// SendMessage allows a manager to send a message to a user on any platform.
func SendMessage(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		platform := chi.URLParam(r, "platform")
		userID := chi.URLParam(r, "user_id")

		if platform == "" || userID == "" {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("platform and user_id are required"))
			return
		}

		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("text is required"))
			return
		}

		err := handler.SendCrmMessage(platform, userID, req.Text)
		if err != nil {
			log.Error("failed to send CRM message",
				slog.String("platform", platform),
				slog.String("user_id", userID),
				slog.String("error", err.Error()),
			)
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("Failed to send message"))
			return
		}

		render.JSON(w, r, response.Ok("message sent"))
	}
}
