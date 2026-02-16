package entity

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MaxFileSize is the maximum allowed file size for uploads (2 MB).
const MaxFileSize = 2 << 20

// Attachment represents a file attached to a ChatMessage.
// The URL field is computed at read-time and not stored in MongoDB.
type Attachment struct {
	FileID   primitive.ObjectID `json:"fileId" bson:"file_id"`
	Filename string             `json:"filename" bson:"filename"`
	MIMEType string             `json:"mimeType" bson:"mime_type"`
	Size     int64              `json:"size" bson:"size"`
	URL      string             `json:"url,omitempty" bson:"-"`
}

// FileMetadata holds GridFS metadata for an uploaded file.
type FileMetadata struct {
	MIMEType string `bson:"mime_type"`
	Platform string `bson:"platform"`
	UserID   string `bson:"user_id"`
	Uploader string `bson:"uploader"` // "user" | "manager"
}
