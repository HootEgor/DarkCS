package crm

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"DarkCS/entity"
	"DarkCS/internal/lib/api/cont"
	"DarkCS/internal/lib/api/response"
)

// maxHistoryExportSize caps an uploaded Telegram Desktop export (text-only exports are small).
const maxHistoryExportSize = 20 << 20

// GetBusinessAccounts lists the Telegram Business (premium) accounts connected to the bot.
// Endpoint: GET /api/v1/crm/business-accounts
func GetBusinessAccounts(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accounts, err := handler.ListBusinessAccounts()
		if err != nil {
			log.Error("failed to list business accounts", slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("Failed to get business accounts"))
			return
		}
		if accounts == nil {
			accounts = []entity.BusinessConnection{}
		}
		render.JSON(w, r, response.Ok(accounts))
	}
}

// GetHistoryStatus reports whether older history of a chat can be (or was) imported.
// Endpoint: GET /api/v1/crm/chats/{platform}/{user_id}/history-status?channel=
func GetHistoryStatus(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		platform := chi.URLParam(r, "platform")
		userID := chi.URLParam(r, "user_id")

		status, err := handler.GetHistoryStatus(platform, userID, chatChannel(r))
		if errors.Is(err, entity.ErrInvalidHistoryImport) {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error(err.Error()))
			return
		}
		if err != nil {
			log.Error("failed to get history status",
				slog.String("platform", platform),
				slog.String("user_id", userID),
				slog.String("error", err.Error()),
			)
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("Failed to get history status"))
			return
		}
		render.JSON(w, r, response.Ok(status))
	}
}

// ImportHistory imports a Telegram Desktop JSON export (result.json) into a chat.
// Endpoint: POST /api/v1/crm/chats/{platform}/{user_id}/import-history?channel=
// Content-Type: multipart/form-data, field "file".
func ImportHistory(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		platform := chi.URLParam(r, "platform")
		userID := chi.URLParam(r, "user_id")

		r.Body = http.MaxBytesReader(w, r.Body, maxHistoryExportSize+1<<20)
		if err := r.ParseMultipartForm(maxHistoryExportSize); err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("invalid multipart form or file larger than 20 MB"))
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("file is required"))
			return
		}
		defer file.Close()
		if header.Size > maxHistoryExportSize {
			render.Status(r, http.StatusRequestEntityTooLarge)
			render.JSON(w, r, response.Error("file larger than 20 MB; export without media"))
			return
		}

		username := cont.GetUser(r.Context()).Username
		result, err := handler.ImportChatHistory(username, platform, userID, chatChannel(r), file)
		if errors.Is(err, entity.ErrInvalidHistoryImport) {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error(err.Error()))
			return
		}
		if err != nil {
			log.Error("failed to import chat history",
				slog.String("platform", platform),
				slog.String("user_id", userID),
				slog.String("error", err.Error()),
			)
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("Failed to import history"))
			return
		}
		render.JSON(w, r, response.Ok(result))
	}
}
