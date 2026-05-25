package tunnel

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"
)

const DefaultRelayPort = 7835

type relayHandshake struct {
	Role string `json:"role"`
	Code string `json:"code,omitempty"`
}

type relayAck struct {
	Code  string `json:"code,omitempty"`
	OK    bool   `json:"ok,omitempty"`
	Error string `json:"error,omitempty"`
}

type pendingRelay struct {
	conn net.Conn
	ch   chan net.Conn
}

// RunRelay starts a self-hosted relay server on addr (e.g. ":7835").
// Sender connects first and receives a code. Receiver connects with the code
// and the relay bridges the two connections for raw TCP passthrough.
func RunRelay(addr string) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("relay listen: %w", err)
	}
	defer l.Close()
	fmt.Printf("Relay listening on %s\n", addr)

	var mu sync.Mutex
	pending := map[string]*pendingRelay{}

	for {
		conn, err := l.Accept()
		if err != nil {
			return fmt.Errorf("relay accept: %w", err)
		}
		go handleRelayConn(conn, &mu, pending)
	}
}

func handleRelayConn(conn net.Conn, mu *sync.Mutex, pending map[string]*pendingRelay) {
	var msg relayHandshake
	if err := relayRead(conn, &msg); err != nil {
		conn.Close()
		return
	}

	switch msg.Role {
	case "sender":
		code := randomCode()
		ch := make(chan net.Conn, 1)

		mu.Lock()
		pending[code] = &pendingRelay{conn: conn, ch: ch}
		mu.Unlock()

		if err := relayWrite(conn, relayAck{Code: code}); err != nil {
			conn.Close()
			mu.Lock()
			delete(pending, code)
			mu.Unlock()
			return
		}

		// Wait for receiver to connect (timeout after 10 minutes)
		select {
		case receiverConn := <-ch:
			// Signal sender that receiver has connected
			if err := relayWrite(conn, relayAck{OK: true}); err != nil {
				conn.Close()
				receiverConn.Close()
				return
			}
			pipe(conn, receiverConn)
		case <-time.After(10 * time.Minute):
			conn.Close()
			mu.Lock()
			delete(pending, code)
			mu.Unlock()
		}

	case "receiver":
		mu.Lock()
		p, ok := pending[msg.Code]
		if ok {
			delete(pending, msg.Code)
		}
		mu.Unlock()

		if !ok {
			relayWrite(conn, relayAck{Error: "unknown or expired code"})
			conn.Close()
			return
		}

		if err := relayWrite(conn, relayAck{OK: true}); err != nil {
			conn.Close()
			return
		}

		p.ch <- conn

	default:
		relayWrite(conn, relayAck{Error: "role must be sender or receiver"})
		conn.Close()
	}
}

// ConnectAsSender connects to a self-hosted relay as sender.
// Returns the raw conn (ready for Noise handshake after receiver connects) and the session code.
func ConnectAsSender(relayAddr string) (net.Conn, string, error) {
	conn, err := net.DialTimeout("tcp", relayAddr, 10*time.Second)
	if err != nil {
		return nil, "", fmt.Errorf("connect to relay: %w", err)
	}

	if err := relayWrite(conn, relayHandshake{Role: "sender"}); err != nil {
		conn.Close()
		return nil, "", fmt.Errorf("send role: %w", err)
	}

	var ack relayAck
	if err := relayRead(conn, &ack); err != nil {
		conn.Close()
		return nil, "", fmt.Errorf("receive code: %w", err)
	}
	if ack.Error != "" {
		conn.Close()
		return nil, "", fmt.Errorf("relay: %s", ack.Error)
	}

	return conn, ack.Code, nil
}

// WaitForReceiver blocks until the relay signals that a receiver has connected.
// After it returns, conn is ready for a raw Noise handshake.
func WaitForReceiver(conn net.Conn) error {
	var ack relayAck
	if err := relayRead(conn, &ack); err != nil {
		return fmt.Errorf("wait for receiver: %w", err)
	}
	if ack.Error != "" {
		return fmt.Errorf("relay: %s", ack.Error)
	}
	return nil
}

// ConnectAsReceiver connects to a self-hosted relay as receiver using a code.
// Returns the raw conn ready for a Noise handshake.
func ConnectAsReceiver(relayAddr, code string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", relayAddr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect to relay: %w", err)
	}

	if err := relayWrite(conn, relayHandshake{Role: "receiver", Code: code}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send code: %w", err)
	}

	var ack relayAck
	if err := relayRead(conn, &ack); err != nil {
		conn.Close()
		return nil, fmt.Errorf("receive ack: %w", err)
	}
	if ack.Error != "" {
		conn.Close()
		return nil, fmt.Errorf("relay: %s", ack.Error)
	}

	return conn, nil
}

func relayWrite(conn net.Conn, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = conn.Write(append(data, '\n'))
	return err
}

func relayRead(conn net.Conn, v interface{}) error {
	var line []byte
	buf := make([]byte, 1)
	for {
		_, err := conn.Read(buf)
		if err != nil {
			return err
		}
		if buf[0] == '\n' {
			break
		}
		line = append(line, buf[0])
	}
	return json.Unmarshal(line, v)
}

func randomCode() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}
