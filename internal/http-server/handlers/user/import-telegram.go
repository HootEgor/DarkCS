package user

import (
	"DarkCS/entity"
	"encoding/json"
	"log/slog"
	"net/http"
)

func ImportTelegram(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var items []entity.TelegramImportItem
		if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		processed, err := handler.ImportTelegramUsers(items)
		if err != nil {
			log.Error("Failed to import telegram users", slog.Any("error", err))
			http.Error(w, "Failed to import telegram users", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"processed": processed})
	}
}
