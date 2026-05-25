package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

const FileChunkSize = 32 * 1024

const (
	MessageTypeFileStart    = "file_start"
	MessageTypeFileChunk    = "file_chunk"
	MessageTypeFileComplete = "file_complete"
	MessageTypeFileChecksum = "file_checksum"
	MessageTypeReady        = "ready"
)

type Message struct {
	Type     string `json:"type"`
	FileID   string `json:"file_id,omitempty"`
	Name     string `json:"name,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Index    int    `json:"index,omitempty"`
	Chunk    []byte `json:"chunk,omitempty"`
	Checksum string `json:"checksum,omitempty"`
}

func EncodeMessage(message Message) ([]byte, error) {
	if err := ValidateMessage(message); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("marshal protocol message: %w", err)
	}
	return payload, nil
}

func DecodeMessage(payload []byte) (Message, error) {
	var message Message
	if err := json.Unmarshal(payload, &message); err != nil {
		return Message{}, fmt.Errorf("unmarshal protocol message: %w", err)
	}
	if err := ValidateMessage(message); err != nil {
		return Message{}, err
	}
	return message, nil
}

func ValidateMessage(message Message) error {
	switch message.Type {
	case MessageTypeFileStart:
		if message.FileID == "" || message.Name == "" || message.Size < -1 {
			return errors.New("file_start requires file_id, name, and size >= -1")
		}
	case MessageTypeFileChunk:
		if message.FileID == "" || len(message.Chunk) == 0 || message.Index < 0 {
			return errors.New("file_chunk requires file_id, chunk bytes, and non-negative index")
		}
	case MessageTypeFileComplete:
		if message.FileID == "" {
			return errors.New("file_complete requires file_id")
		}
	case MessageTypeFileChecksum:
		if message.FileID == "" || message.Checksum == "" {
			return errors.New("file_checksum requires file_id and checksum")
		}
	case MessageTypeReady:
		// no fields required
	default:
		return fmt.Errorf("unknown protocol message type %q", message.Type)
	}
	return nil
}
