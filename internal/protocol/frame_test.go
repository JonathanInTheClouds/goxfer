package protocol

import (
	"bytes"
	"net"
	"testing"
)

func TestEncodeDecodeFrame_RoundTrip(t *testing.T) {
	payloads := [][]byte{
		[]byte("hello"),
		make([]byte, MaxFrameSize),
		{},
	}
	for _, payload := range payloads {
		a, b := net.Pipe()
		go func() {
			frame, err := EncodeFrame(payload)
			if err != nil {
				b.Close()
				return
			}
			b.Write(frame)
			b.Close()
		}()
		got, err := DecodeFrame(a)
		a.Close()
		if err != nil {
			t.Fatalf("DecodeFrame: %v (payload len %d)", err, len(payload))
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("roundtrip mismatch: got len %d, want len %d", len(got), len(payload))
		}
	}
}

func TestEncodeFrame_Oversized(t *testing.T) {
	_, err := EncodeFrame(make([]byte, MaxFrameSize+1))
	if err == nil {
		t.Fatal("expected error for oversized payload, got nil")
	}
}

func TestDecodeFrame_Oversized(t *testing.T) {
	// Craft a frame header claiming an oversized payload
	header := []byte{0xFF, 0xFF, 0xFF, 0xFF} // > MaxFrameSize
	a, b := net.Pipe()
	go func() {
		b.Write(header)
		b.Close()
	}()
	_, err := DecodeFrame(a)
	a.Close()
	if err == nil {
		t.Fatal("expected error for oversized frame header, got nil")
	}
}
