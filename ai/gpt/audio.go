package gpt

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"github.com/sashabaranov/go-openai"
	"io"
	"os"
)

func (o *Overseer) GetAudioText(base64 string) (string, error) {

	audioData, err := o.base64Decode(base64)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 audio: %w", err)
	}

	tmpFile, err := os.CreateTemp(o.savePath, "audio_*.mp3")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, audioData)
	if err != nil {
		return "", fmt.Errorf("failed to copy audio to file: %w", err)
	}

	transcription, err := o.transcribeAudio(tmpFile.Name())
	if err != nil {
		return "", fmt.Errorf("failed to transcribe audio: %w", err)
	}

	return transcription, nil
}

// maxAudioBytes matches the Whisper API upload limit; larger input is rejected before
// decoding so a request cannot force a huge allocation.
const maxAudioBytes = 25 << 20

// base64Decode decodes client-supplied audio. Invalid input is returned as an error:
// this runs in the request path, where exiting the process would take every bot down.
func (o *Overseer) base64Decode(base64Str string) (io.Reader, error) {
	if base64.StdEncoding.DecodedLen(len(base64Str)) > maxAudioBytes {
		return nil, fmt.Errorf("audio exceeds %d bytes", maxAudioBytes)
	}

	decoded, err := base64.StdEncoding.DecodeString(base64Str)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(decoded), nil
}

func (o *Overseer) transcribeAudio(filePath string) (string, error) {

	req := openai.AudioRequest{
		Model:    openai.Whisper1,
		FilePath: filePath,
		Format:   openai.AudioResponseFormatText,
	}

	resp, err := o.client.CreateTranscription(context.Background(), req)
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}
