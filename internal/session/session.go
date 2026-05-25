package session

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/JonathanInTheClouds/goxfer/internal/crypto"
	"github.com/JonathanInTheClouds/goxfer/internal/protocol"
	"github.com/flynn/noise"
)

const prologue = "github.com/JonathanInTheClouds/goxfer/v1"

type Listener struct {
	inner    net.Listener
	identity *crypto.Identity
}

type SecureSession struct {
	conn    net.Conn
	send    *noise.CipherState
	receive *noise.CipherState
}

// Bind starts a TCP listener on addr (use ":0" to let the OS pick a port).
// Returns the Listener and the actual port bound.
func Bind(addr string, identity *crypto.Identity) (*Listener, int, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, 0, fmt.Errorf("listen: %w", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	return &Listener{inner: l, identity: identity}, port, nil
}

// Accept blocks until one peer connects, then runs the Noise XX handshake as responder.
func (l *Listener) Accept() (*SecureSession, error) {
	conn, err := l.inner.Accept()
	if err != nil {
		return nil, fmt.Errorf("accept: %w", err)
	}
	sess, err := newSession(conn, l.identity, false)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return sess, nil
}

func (l *Listener) Close() error {
	return l.inner.Close()
}

// Dial connects to addr and runs the Noise XX handshake as initiator.
func Dial(addr string, identity *crypto.Identity) (*SecureSession, error) {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	sess, err := newSession(conn, identity, true)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return sess, nil
}

// NewSession runs the Noise XX handshake over an existing connection.
// Use initiator=true for the receiver side, initiator=false for the sender side.
func NewSession(conn net.Conn, identity *crypto.Identity, initiator bool) (*SecureSession, error) {
	sess, err := newSession(conn, identity, initiator)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return sess, nil
}

func newSession(conn net.Conn, identity *crypto.Identity, initiator bool) (*SecureSession, error) {
	handshake, err := noise.NewHandshakeState(noise.Config{
		CipherSuite:   noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashSHA256),
		Pattern:       noise.HandshakeXX,
		Initiator:     initiator,
		StaticKeypair: identity.NoiseStaticKeypair(),
	})
	if err != nil {
		return nil, fmt.Errorf("create handshake state: %w", err)
	}

	sendCipher, receiveCipher, err := runHandshake(conn, handshake, initiator)
	if err != nil {
		return nil, err
	}
	if !initiator {
		sendCipher, receiveCipher = receiveCipher, sendCipher
	}

	return &SecureSession{
		conn:    conn,
		send:    sendCipher,
		receive: receiveCipher,
	}, nil
}

func (s *SecureSession) SendMessage(msg protocol.Message) error {
	payload, err := protocol.EncodeMessage(msg)
	if err != nil {
		return err
	}
	ciphertext, err := s.send.Encrypt(nil, nil, payload)
	if err != nil {
		return fmt.Errorf("encrypt message: %w", err)
	}
	frame, err := protocol.EncodeFrame(ciphertext)
	if err != nil {
		return err
	}
	if _, err := s.conn.Write(frame); err != nil {
		return fmt.Errorf("write encrypted frame: %w", err)
	}
	return nil
}

func (s *SecureSession) ReceiveMessage() (protocol.Message, error) {
	frame, err := protocol.DecodeFrame(s.conn)
	if err != nil {
		return protocol.Message{}, fmt.Errorf("read encrypted frame: %w", err)
	}
	plaintext, err := s.receive.Decrypt(nil, nil, frame)
	if err != nil {
		return protocol.Message{}, fmt.Errorf("decrypt message: %w", err)
	}
	return protocol.DecodeMessage(plaintext)
}

func (s *SecureSession) Close() error {
	return s.conn.Close()
}

func runHandshake(conn net.Conn, handshake *noise.HandshakeState, initiator bool) (*noise.CipherState, *noise.CipherState, error) {
	var (
		writeCipher *noise.CipherState
		readCipher  *noise.CipherState
		err         error
	)

	if initiator {
		if writeCipher, readCipher, err = writeHandshakeMessage(conn, handshake, []byte(prologue)); err != nil {
			return nil, nil, err
		}
		if writeCipher == nil && readCipher == nil {
			if writeCipher, readCipher, err = readHandshakeMessage(conn, handshake); err != nil {
				return nil, nil, err
			}
		}
		if writeCipher == nil && readCipher == nil {
			if writeCipher, readCipher, err = writeHandshakeMessage(conn, handshake, nil); err != nil {
				return nil, nil, err
			}
		}
	} else {
		if writeCipher, readCipher, err = readHandshakeMessage(conn, handshake); err != nil {
			return nil, nil, err
		}
		if writeCipher == nil && readCipher == nil {
			if writeCipher, readCipher, err = writeHandshakeMessage(conn, handshake, []byte(prologue)); err != nil {
				return nil, nil, err
			}
		}
		if writeCipher == nil && readCipher == nil {
			if writeCipher, readCipher, err = readHandshakeMessage(conn, handshake); err != nil {
				return nil, nil, err
			}
		}
	}

	if writeCipher == nil || readCipher == nil {
		return nil, nil, errors.New("handshake did not produce transport cipher states")
	}

	return writeCipher, readCipher, nil
}

func writeHandshakeMessage(conn net.Conn, handshake *noise.HandshakeState, payload []byte) (*noise.CipherState, *noise.CipherState, error) {
	message, sendCipher, recvCipher, err := handshake.WriteMessage(nil, payload)
	if err != nil {
		return nil, nil, fmt.Errorf("write handshake message: %w", err)
	}
	frame, err := protocol.EncodeFrame(message)
	if err != nil {
		return nil, nil, err
	}
	if _, err := conn.Write(frame); err != nil {
		return nil, nil, fmt.Errorf("send handshake frame: %w", err)
	}
	return sendCipher, recvCipher, nil
}

func readHandshakeMessage(conn net.Conn, handshake *noise.HandshakeState) (*noise.CipherState, *noise.CipherState, error) {
	frame, err := protocol.DecodeFrame(conn)
	if err != nil {
		return nil, nil, fmt.Errorf("read handshake frame: %w", err)
	}
	_, sendCipher, recvCipher, err := handshake.ReadMessage(nil, frame)
	if err != nil {
		return nil, nil, fmt.Errorf("read handshake message: %w", err)
	}
	return sendCipher, recvCipher, nil
}
