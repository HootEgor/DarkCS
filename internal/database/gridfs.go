package repository

import (
	"fmt"
	"io"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/gridfs"
	"go.mongodb.org/mongo-driver/mongo/options"

	"DarkCS/entity"
)

// UploadFile stores a file in GridFS and returns the generated file ID and size.
func (m *MongoDB) UploadFile(filename string, reader io.Reader, meta entity.FileMetadata) (primitive.ObjectID, int64, error) {
	connection, err := m.connect()
	if err != nil {
		return primitive.NilObjectID, 0, err
	}
	defer m.disconnect(connection)

	bucket, err := gridfs.NewBucket(connection.Database(m.database))
	if err != nil {
		return primitive.NilObjectID, 0, fmt.Errorf("gridfs bucket: %w", err)
	}

	uploadOpts := options.GridFSUpload().SetMetadata(meta)
	uploadStream, err := bucket.OpenUploadStream(filename, uploadOpts)
	if err != nil {
		return primitive.NilObjectID, 0, fmt.Errorf("gridfs open upload: %w", err)
	}

	size, err := io.Copy(uploadStream, reader)
	if err != nil {
		uploadStream.Close()
		return primitive.NilObjectID, 0, fmt.Errorf("gridfs copy: %w", err)
	}

	if err := uploadStream.Close(); err != nil {
		return primitive.NilObjectID, 0, fmt.Errorf("gridfs close upload: %w", err)
	}

	fileID := uploadStream.FileID.(primitive.ObjectID)
	return fileID, size, nil
}

// gridfsReadCloser wraps a GridFS download stream and disconnects
// the MongoDB client when closed.
type gridfsReadCloser struct {
	stream     *gridfs.DownloadStream
	disconnect func()
}

func (r *gridfsReadCloser) Read(p []byte) (int, error) {
	return r.stream.Read(p)
}

func (r *gridfsReadCloser) Close() error {
	err := r.stream.Close()
	r.disconnect()
	return err
}

// DownloadFile retrieves a file from GridFS by its ID.
// The caller must close the returned ReadCloser to release the MongoDB connection.
func (m *MongoDB) DownloadFile(fileID primitive.ObjectID) (string, entity.FileMetadata, io.ReadCloser, error) {
	connection, err := m.connect()
	if err != nil {
		return "", entity.FileMetadata{}, nil, err
	}

	bucket, err := gridfs.NewBucket(connection.Database(m.database))
	if err != nil {
		m.disconnect(connection)
		return "", entity.FileMetadata{}, nil, fmt.Errorf("gridfs bucket: %w", err)
	}

	stream, err := bucket.OpenDownloadStream(fileID)
	if err != nil {
		m.disconnect(connection)
		return "", entity.FileMetadata{}, nil, fmt.Errorf("gridfs open download: %w", err)
	}

	file := stream.GetFile()
	filename := file.Name

	var meta entity.FileMetadata
	if len(file.Metadata) > 0 {
		if err := bson.Unmarshal(file.Metadata, &meta); err != nil {
			m.log.Error("failed to unmarshal gridfs metadata", "error", err.Error())
		}
	}

	reader := &gridfsReadCloser{
		stream:     stream,
		disconnect: func() { m.disconnect(connection) },
	}

	return filename, meta, reader, nil
}
