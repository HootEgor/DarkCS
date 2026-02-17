package crm

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"DarkCS/internal/lib/fileurl"
)

// DownloadFile streams a file from GridFS to the HTTP response.
// Endpoint: GET /api/v1/crm/files/{file_id}
// Auth is via HMAC-signed URL: ?expires={unix}&sig={hmac_hex}.
func DownloadFile(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fileIDStr := chi.URLParam(r, "file_id")
		expires := r.URL.Query().Get("expires")
		sig := r.URL.Query().Get("sig")

		if expires == "" || sig == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if !fileurl.Verify(fileIDStr, expires, sig, handler.FileSigningSecret()) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

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
