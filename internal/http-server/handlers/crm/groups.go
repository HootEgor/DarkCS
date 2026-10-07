package crm

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"DarkCS/entity"
	"DarkCS/internal/lib/api/cont"
	"DarkCS/internal/lib/api/response"
)

// Telegram groups review. Groups the bot is added to wait as "pending" until a manager
// approves them here; group chats themselves use the regular /crm/chats routes with
// platform telegram_group, user_id = group chat id and ?channel= topic id.

// GetGroups lists Telegram groups, optionally filtered by status.
// Endpoint: GET /api/v1/crm/groups?status=pending|approved|rejected|left
func GetGroups(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		switch status {
		case "", entity.GroupStatusPending, entity.GroupStatusApproved, entity.GroupStatusRejected, entity.GroupStatusLeft:
		default:
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("invalid status"))
			return
		}
		groups, err := handler.ListTelegramGroups(status)
		if err != nil {
			log.Error("failed to list telegram groups", slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("Failed to get groups"))
			return
		}
		if groups == nil {
			groups = []entity.TelegramGroup{}
		}
		render.JSON(w, r, response.Ok(groups))
	}
}

// ApproveGroup starts relaying a group to the CRM.
// Endpoint: POST /api/v1/crm/groups/{chat_id}/approve
func ApproveGroup(log *slog.Logger, handler Core) http.HandlerFunc {
	return reviewGroup(log, handler.ApproveGroup)
}

// RejectGroup stops relaying a group and makes the bot leave it.
// Endpoint: POST /api/v1/crm/groups/{chat_id}/reject
func RejectGroup(log *slog.Logger, handler Core) http.HandlerFunc {
	return reviewGroup(log, handler.RejectGroup)
}

func reviewGroup(log *slog.Logger, review func(chatID int64, username string) (*entity.TelegramGroup, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chatID, ok := groupChatID(w, r)
		if !ok {
			return
		}
		group, err := review(chatID, cont.GetUser(r.Context()).Username)
		if errors.Is(err, entity.ErrGroupNotFound) {
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, response.Error(err.Error()))
			return
		}
		if err != nil {
			log.Error("failed to review telegram group", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("Failed to update group"))
			return
		}
		render.JSON(w, r, response.Ok(group))
	}
}

// GetGroupTopics lists the known forum topics of a group.
// Endpoint: GET /api/v1/crm/groups/{chat_id}/topics
func GetGroupTopics(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chatID, ok := groupChatID(w, r)
		if !ok {
			return
		}
		topics, err := handler.ListGroupTopics(chatID)
		if err != nil {
			log.Error("failed to list group topics", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("Failed to get topics"))
			return
		}
		if topics == nil {
			topics = []entity.TelegramGroupTopic{}
		}
		render.JSON(w, r, response.Ok(topics))
	}
}

// RenameGroupTopic names a topic in the CRM (for topics created before the bot joined).
// Endpoint: PUT /api/v1/crm/groups/{chat_id}/topics/{thread_id}   {"name": "..."}
func RenameGroupTopic(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chatID, ok := groupChatID(w, r)
		if !ok {
			return
		}
		threadID, err := strconv.ParseInt(chi.URLParam(r, "thread_id"), 10, 64)
		if err != nil || threadID <= 0 {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("invalid thread_id"))
			return
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("name is required"))
			return
		}
		err = handler.RenameGroupTopic(chatID, threadID, req.Name)
		if errors.Is(err, entity.ErrGroupNotFound) {
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, response.Error(err.Error()))
			return
		}
		if err != nil {
			log.Error("failed to rename group topic", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("Failed to rename topic"))
			return
		}
		render.JSON(w, r, response.Ok("topic renamed"))
	}
}

// groupChatID parses the {chat_id} URL param (negative for groups) or writes a 400.
func groupChatID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	chatID, err := strconv.ParseInt(chi.URLParam(r, "chat_id"), 10, 64)
	if err != nil || chatID == 0 {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, response.Error("invalid chat_id"))
		return 0, false
	}
	return chatID, true
}
