package crm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"DarkCS/entity"
	"DarkCS/internal/lib/api/cont"
	"DarkCS/internal/lib/api/response"
)

// Core defines the methods required by CRM handlers.
type Core interface {
	GetActiveChats(username string) ([]entity.ChatSummary, error)
	GetChatMessages(platform, userID, channel string, limit, offset int) ([]entity.ChatMessage, error)
	ListMessagesSince(ctx context.Context, since time.Time, afterID primitive.ObjectID, platforms []string, limit int) ([]entity.ChatMessage, error)
	SendCrmMessage(platform, userID, channel, text, operatorID string) error
	DownloadFile(fileID primitive.ObjectID) (filename, mimeType string, reader io.ReadCloser, err error)
	UploadFile(filename string, reader io.Reader, meta entity.FileMetadata) (primitive.ObjectID, int64, error)
	SendCrmFiles(platform, userID, channel, caption, operatorID string, attachments []entity.Attachment) error
	FileSigningSecret() string
	IssueWsTicket(username string) (string, error)
	ListBusinessAccounts() ([]entity.BusinessConnection, error)
	GetHistoryStatus(platform, userID, channel string) (*entity.ChatHistoryStatus, error)
	ImportChatHistory(username, platform, userID, channel string, r io.Reader) (*entity.ChatHistoryImportResult, error)
	ListTelegramGroups(status string) ([]entity.TelegramGroup, error)
	ApproveGroup(chatID int64, username string) (*entity.TelegramGroup, error)
	RejectGroup(chatID int64, username string) (*entity.TelegramGroup, error)
	ListGroupTopics(chatID int64) ([]entity.TelegramGroupTopic, error)
	RenameGroupTopic(chatID, threadID int64, name string) error
}

// isChannelUnavailable reports errors meaning the chat cannot be written to right now
// (premium account disconnected, group not approved or bot removed); their text is
// meant for the manager and they map to 409.
func isChannelUnavailable(err error) bool {
	return errors.Is(err, entity.ErrChannelUnavailable) || errors.Is(err, entity.ErrGroupUnavailable)
}

// chatChannel reads the optional ?channel= query that, with platform and user_id,
// identifies a chat. Only Telegram Business chats (the premium account owner id) and
// group chats (the forum topic id) have one; clients that never send it keep addressing
// the default channel.
func chatChannel(r *http.Request) string {
	return r.URL.Query().Get("channel")
}

// Operator identity sent by the CRM on replies. The header is optional (older clients
// and integrations send none); a value that is too long or has characters outside the
// allowed set is dropped rather than rejected, so a bad header never blocks a send.
const (
	operatorIDHeader = "X-Operator-Id"
	maxOperatorIDLen = 64
)

var operatorIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// operatorID returns the validated X-Operator-Id header value, or "" when absent or invalid.
func operatorID(r *http.Request) string {
	v := strings.TrimSpace(r.Header.Get(operatorIDHeader))
	if v == "" || len(v) > maxOperatorIDLen || !operatorIDPattern.MatchString(v) {
		return ""
	}
	return v
}

// Incremental listing limits (GET /crm/messages).
const (
	defaultSinceLimit = 200
	maxSinceLimit     = 500
)

// messagesSincePage is the data of GET /crm/messages: a page of changed messages and the
// cursor to continue from. Next is nil when the page is empty.
type messagesSincePage struct {
	Messages []entity.ChatMessage `json:"messages"`
	Next     *messagesCursor      `json:"next"`
	HasMore  bool                 `json:"has_more"`
}

// messagesCursor is passed back as ?since=&after_id= to continue after the last row of a
// page. Since is RFC3339 with fractional seconds: Mongo stores milliseconds, and a
// cursor rounded to whole seconds would re-read the last second's rows every pull.
type messagesCursor struct {
	Since   string `json:"since"`
	AfterID string `json:"after_id"`
}

// ListMessages returns messages changed since a cursor, across all chats, for external
// consumers that mirror the message store incrementally (edits and soft deletes included).
// Endpoint: GET /api/v1/crm/messages?since=RFC3339[&after_id=hex][&limit=200][&platforms=a,b]
func ListMessages(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		sinceRaw := q.Get("since")
		if sinceRaw == "" {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("since is required (RFC3339)"))
			return
		}
		since, err := time.Parse(time.RFC3339Nano, sinceRaw)
		if err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("since must be an RFC3339 time"))
			return
		}

		var afterID primitive.ObjectID
		if raw := q.Get("after_id"); raw != "" {
			afterID, err = primitive.ObjectIDFromHex(raw)
			if err != nil {
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, response.Error("after_id must be a hex ObjectID"))
				return
			}
		}

		limit := defaultSinceLimit
		if l := q.Get("limit"); l != "" {
			v, err := strconv.Atoi(l)
			if err != nil || v <= 0 {
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, response.Error("limit must be a positive integer"))
				return
			}
			limit = min(v, maxSinceLimit)
		}

		var platforms []string
		for _, p := range strings.Split(q.Get("platforms"), ",") {
			if p = strings.TrimSpace(p); p != "" {
				platforms = append(platforms, p)
			}
		}

		messages, err := handler.ListMessagesSince(r.Context(), since, afterID, platforms, limit)
		if err != nil {
			log.Error("failed to list messages since",
				slog.String("since", sinceRaw),
				slog.String("after_id", afterID.Hex()),
				slog.String("error", err.Error()),
			)
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("Failed to list messages"))
			return
		}

		page := messagesSincePage{Messages: messages, HasMore: len(messages) == limit}
		if page.Messages == nil {
			page.Messages = []entity.ChatMessage{}
		}
		if n := len(messages); n > 0 {
			last := messages[n-1]
			page.Next = &messagesCursor{
				Since:   last.UpdatedAt.UTC().Format(time.RFC3339Nano),
				AfterID: last.ID.Hex(),
			}
		}

		render.JSON(w, r, response.Ok(page))
	}
}

// IssueWsTicket returns a one-time ticket for opening the CRM socket as
// /api/v1/crm/ws?ticket=..., so the browser doesn't put the API key in the socket URL.
func IssueWsTicket(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := cont.GetUser(r.Context()).Username
		ticket, err := handler.IssueWsTicket(username)
		if err != nil {
			log.Error("failed to issue ws ticket", slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("failed to issue ticket"))
			return
		}
		render.JSON(w, r, response.Ok(map[string]string{"ticket": ticket}))
	}
}

// GetChats returns the list of active chats with last message info.
func GetChats(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := cont.GetUser(r.Context()).Username
		chats, err := handler.GetActiveChats(username)
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

		messages, err := handler.GetChatMessages(platform, userID, chatChannel(r), limit, offset)
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

		err := handler.SendCrmMessage(platform, userID, chatChannel(r), req.Text, operatorID(r))
		if isChannelUnavailable(err) {
			render.Status(r, http.StatusConflict)
			render.JSON(w, r, response.Error(err.Error()))
			return
		}
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
