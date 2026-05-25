package protocol

import (
	"testing"
)

func TestEncodeDecodeMessage_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
	}{
		{
			name: "file_start",
			msg:  Message{Type: MessageTypeFileStart, FileID: "abc123", Name: "test.txt", Size: 1024},
		},
		{
			name: "file_start streaming",
			msg:  Message{Type: MessageTypeFileStart, FileID: "abc123", Name: "dir.tar.gz", Size: -1},
		},
		{
			name: "file_chunk",
			msg:  Message{Type: MessageTypeFileChunk, FileID: "abc123", Index: 0},
		},
		{
			name: "file_complete",
			msg:  Message{Type: MessageTypeFileComplete, FileID: "abc123"},
		},
		{
			name: "file_checksum",
			msg:  Message{Type: MessageTypeFileChecksum, FileID: "abc123", Checksum: "deadbeef"},
		},
		{
			name: "ready",
			msg:  Message{Type: MessageTypeReady},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := EncodeMessage(tt.msg)
			if err != nil {
				t.Fatalf("EncodeMessage: %v", err)
			}
			got, err := DecodeMessage(encoded)
			if err != nil {
				t.Fatalf("DecodeMessage: %v", err)
			}
			if got.Type != tt.msg.Type || got.FileID != tt.msg.FileID {
				t.Fatalf("roundtrip mismatch: got %+v, want %+v", got, tt.msg)
			}
		})
	}
}

func TestEncodeMessage_ChunkNotInJSON(t *testing.T) {
	msg := Message{Type: MessageTypeFileChunk, FileID: "abc", Index: 0, Chunk: []byte("secret")}
	encoded, err := EncodeMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	// Chunk data must not appear in the JSON output (json:"-" tag)
	if contains(encoded, []byte("secret")) {
		t.Fatal("chunk data leaked into JSON encoding")
	}
	// Check for the JSON key "chunk" (with quotes), not just the substring "chunk"
	// which would falsely match the "file_chunk" type value.
	if contains(encoded, []byte(`"chunk"`)) {
		t.Fatal("chunk field appeared as JSON key in encoding")
	}
}

func TestValidateMessage_Invalid(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
	}{
		{"unknown type", Message{Type: "bogus"}},
		{"file_start missing name", Message{Type: MessageTypeFileStart, FileID: "x", Size: 0}},
		{"file_start missing file_id", Message{Type: MessageTypeFileStart, Name: "f", Size: 0}},
		{"file_start bad size", Message{Type: MessageTypeFileStart, FileID: "x", Name: "f", Size: -2}},
		{"file_chunk missing file_id", Message{Type: MessageTypeFileChunk, Index: 0}},
		{"file_chunk negative index", Message{Type: MessageTypeFileChunk, FileID: "x", Index: -1}},
		{"file_complete missing file_id", Message{Type: MessageTypeFileComplete}},
		{"file_checksum missing checksum", Message{Type: MessageTypeFileChecksum, FileID: "x"}},
		{"file_checksum missing file_id", Message{Type: MessageTypeFileChecksum, Checksum: "abc"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateMessage(tt.msg); err == nil {
				t.Fatalf("expected validation error for %+v, got nil", tt.msg)
			}
		})
	}
}

func contains(haystack, needle []byte) bool {
	for i := 0; i <= len(haystack)-len(needle); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
