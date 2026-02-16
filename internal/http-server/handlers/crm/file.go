package crm

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"DarkCS/internal/http-server/middleware/authenticate"
)

// DownloadFile streams a file from GridFS to the HTTP response.
// Endpoint: GET /api/v1/crm/files/{file_id}
// Accepts auth via Authorization header OR ?token= query param (for <img src> / <a href>).
func DownloadFile(log *slog.Logger, handler Core, auth authenticate.Authenticate) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Authenticate via Bearer header or query param
		token := ""
		if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
			token = header[7:]
		}
		if token == "" {
			token = r.URL.Query().Get("token")
		}
		if token == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if _, err := auth.AuthenticateByToken(token); err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		fileIDStr := chi.URLParam(r, "file_id")
		if fileIDStr == "" {
			http.Error(w, "file_id is required", http.StatusBadRequest)
			return
		}

		fileID, err := primitive.ObjectIDFromHex(fileIDStr)
		if err != nil {
			http.Error(w, "invalid file_id", http.StatusBadRequest)
			return
		}

		filename, mimeType, reader, err := handler.DownloadFile(fileID)
		if err != nil {
			log.Error("failed to download file",
				slog.String("file_id", fileIDStr),
				slog.String("error", err.Error()),
			)
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		defer reader.Close()

		if mimeType != "" {
			w.Header().Set("Content-Type", mimeType)
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))

		if _, err := io.Copy(w, reader); err != nil {
			log.Error("failed to stream file",
				slog.String("file_id", fileIDStr),
				slog.String("error", err.Error()),
			)
		}
	}
}
