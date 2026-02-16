package crm

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"DarkCS/entity"
	"DarkCS/internal/lib/api/response"
)

// SendFile handles file uploads from a CRM manager to a user.
// Endpoint: POST /api/v1/crm/chats/{platform}/{user_id}/send-file
// Content-Type: multipart/form-data
// Fields: files (multiple), caption (optional text)
func SendFile(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		platform := chi.URLParam(r, "platform")
		userID := chi.URLParam(r, "user_id")

		if platform == "" || userID == "" {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("platform and user_id are required"))
			return
		}

		if err := r.ParseMultipartForm(entity.MaxFileSize); err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("invalid multipart form"))
			return
		}

		caption := r.FormValue("caption")
		files := r.MultipartForm.File["files"]

		if len(files) == 0 {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("at least one file is required"))
			return
		}

		// Validate file sizes before uploading
		for _, fh := range files {
			if fh.Size > entity.MaxFileSize {
				render.Status(r, http.StatusRequestEntityTooLarge)
				render.JSON(w, r, response.Error(fmt.Sprintf("file %q exceeds the %d MB limit", fh.Filename, entity.MaxFileSize>>20)))
				return
			}
		}

		var attachments []entity.Attachment
		for _, fh := range files {
			file, err := fh.Open()
			if err != nil {
				log.Error("failed to open uploaded file",
					slog.String("filename", fh.Filename),
					slog.String("error", err.Error()),
				)
				render.Status(r, http.StatusInternalServerError)
				render.JSON(w, r, response.Error("failed to read uploaded file"))
				return
			}

			mimeType := fh.Header.Get("Content-Type")
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}

			meta := entity.FileMetadata{
				MIMEType: mimeType,
				Platform: platform,
				UserID:   userID,
				Uploader: "manager",
			}

			fileID, size, err := handler.UploadFile(fh.Filename, file, meta)
			file.Close()
			if err != nil {
				log.Error("failed to upload file to GridFS",
					slog.String("filename", fh.Filename),
					slog.String("error", err.Error()),
				)
				render.Status(r, http.StatusInternalServerError)
				render.JSON(w, r, response.Error("failed to store file"))
				return
			}

			attachments = append(attachments, entity.Attachment{
				FileID:   fileID,
				Filename: fh.Filename,
				MIMEType: mimeType,
				Size:     size,
			})
		}

		if err := handler.SendCrmFiles(platform, userID, caption, attachments); err != nil {
			log.Error("failed to send files",
				slog.String("platform", platform),
				slog.String("user_id", userID),
				slog.String("error", err.Error()),
			)
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("failed to send files"))
			return
		}

		render.JSON(w, r, response.Ok("files sent"))
	}
}
