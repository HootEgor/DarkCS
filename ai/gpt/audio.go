package gpt

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"github.com/sashabaranov/go-openai"
	"io"
	"log"
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

func (o *Overseer) base64Decode(base64Str string) (io.Reader, error) {

	decoded, err := base64.StdEncoding.DecodeString(base64Str)
	if err != nil {
		log.Fatal(err)
	}

	return bytes.NewBuffer(decoded), nil
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
