package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

const MaxFrameSize = 64 * 1024

func EncodeFrame(payload []byte) ([]byte, error) {
	if len(payload) > MaxFrameSize {
		return nil, fmt.Errorf("payload exceeds max frame size of %d bytes", MaxFrameSize)
	}

	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(payload)))
	copy(frame[4:], payload)
	return frame, nil
}

func DecodeFrame(r io.Reader) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	size := binary.BigEndian.Uint32(header)
	if size > MaxFrameSize {
		return nil, fmt.Errorf("incoming frame exceeds max frame size of %d bytes", MaxFrameSize)
	}

	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}

	return payload, nil
}
