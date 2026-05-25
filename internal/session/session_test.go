package session

import (
	"net"
	"testing"

	"github.com/JonathanInTheClouds/goxfer/internal/crypto"
	"github.com/JonathanInTheClouds/goxfer/internal/protocol"
)

// makeSessions creates a pair of connected SecureSessions using an in-memory pipe.
// sender = Noise responder, receiver = Noise initiator (matches p2p.go convention).
func makeSessions(t *testing.T) (senderSess, receiverSess *SecureSession) {
	t.Helper()
	senderID, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity sender: %v", err)
	}
	receiverID, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity receiver: %v", err)
	}

	connA, connB := net.Pipe()

	type result struct {
		sess *SecureSession
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		s, err := newSession(connA, senderID, false) // responder
		ch <- result{s, err}
	}()

	recvSess, err := newSession(connB, receiverID, true) // initiator
	if err != nil {
		connA.Close()
		connB.Close()
		t.Fatalf("receiver session: %v", err)
	}

	r := <-ch
	if r.err != nil {
		recvSess.Close()
		t.Fatalf("sender session: %v", r.err)
	}

	t.Cleanup(func() {
		r.sess.Close()
		recvSess.Close()
	})

	return r.sess, recvSess
}

func TestNoiseHandshake_RoundTrip(t *testing.T) {
	senderSess, receiverSess := makeSessions(t)

	want := protocol.Message{Type: protocol.MessageTypeReady}

	errCh := make(chan error, 1)
	go func() {
		errCh <- senderSess.SendMessage(want)
	}()

	got, err := receiverSess.ReceiveMessage()
	if err != nil {
		t.Fatalf("ReceiveMessage: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if got.Type != want.Type {
		t.Fatalf("type: got %q, want %q", got.Type, want.Type)
	}
}

func TestBinaryChunkRoundTrip(t *testing.T) {
	senderSess, receiverSess := makeSessions(t)

	chunkData := []byte("binary chunk payload — not base64")
	want := protocol.Message{
		Type:   protocol.MessageTypeFileChunk,
		FileID: "testfile42",
		Index:  7,
		Chunk:  chunkData,
	}

	recvCh := make(chan protocol.Message, 1)
	errCh := make(chan error, 1)
	go func() {
		msg, err := receiverSess.ReceiveMessage()
		errCh <- err
		recvCh <- msg
	}()

	if err := senderSess.SendMessage(want); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("ReceiveMessage: %v", err)
	}

	got := <-recvCh
	if got.Type != want.Type {
		t.Fatalf("type: got %q, want %q", got.Type, want.Type)
	}
	if got.FileID != want.FileID {
		t.Fatalf("file_id: got %q, want %q", got.FileID, want.FileID)
	}
	if got.Index != want.Index {
		t.Fatalf("index: got %d, want %d", got.Index, want.Index)
	}
	if string(got.Chunk) != string(want.Chunk) {
		t.Fatalf("chunk: got %q, want %q", got.Chunk, want.Chunk)
	}
}

func TestBidirectionalMessages(t *testing.T) {
	senderSess, receiverSess := makeSessions(t)

	// sender → receiver
	done := make(chan error, 1)
	go func() {
		done <- senderSess.SendMessage(protocol.Message{Type: protocol.MessageTypeReady})
	}()
	msg1, err := receiverSess.ReceiveMessage()
	if err != nil {
		t.Fatalf("receiver ReceiveMessage: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("sender SendMessage: %v", err)
	}
	if msg1.Type != protocol.MessageTypeReady {
		t.Fatalf("got %q, want %q", msg1.Type, protocol.MessageTypeReady)
	}

	// receiver → sender (checksum ack)
	go func() {
		done <- receiverSess.SendMessage(protocol.Message{
			Type:     protocol.MessageTypeFileChecksum,
			FileID:   "f",
			Checksum: "abc123",
		})
	}()
	msg2, err := senderSess.ReceiveMessage()
	if err != nil {
		t.Fatalf("sender ReceiveMessage: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("receiver SendMessage: %v", err)
	}
	if msg2.Type != protocol.MessageTypeFileChecksum || msg2.Checksum != "abc123" {
		t.Fatalf("got %+v, want file_checksum abc123", msg2)
	}
}

func TestAllMessageTypes(t *testing.T) {
	tests := []protocol.Message{
		{Type: protocol.MessageTypeReady},
		{Type: protocol.MessageTypeFileStart, FileID: "id1", Name: "a.txt", Size: 512},
		{Type: protocol.MessageTypeFileStart, FileID: "id2", Name: "dir.tar.gz", Size: -1},
		{Type: protocol.MessageTypeFileComplete, FileID: "id1"},
		{Type: protocol.MessageTypeFileChecksum, FileID: "id1", Checksum: "deadbeef"},
	}

	for _, want := range tests {
		senderSess, receiverSess := makeSessions(t)

		errCh := make(chan error, 1)
		go func() {
			errCh <- senderSess.SendMessage(want)
		}()

		got, err := receiverSess.ReceiveMessage()
		if err != nil {
			t.Fatalf("[%s] ReceiveMessage: %v", want.Type, err)
		}
		if err := <-errCh; err != nil {
			t.Fatalf("[%s] SendMessage: %v", want.Type, err)
		}

		if got.Type != want.Type || got.FileID != want.FileID {
			t.Fatalf("[%s] got %+v, want %+v", want.Type, got, want)
		}
	}
}
