package tunnel

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
)

const boreServer = "bore.pub:7835"

type boreConn struct {
	net.Conn
	r *bufio.Reader
}

func dial() (*boreConn, error) {
	c, err := net.DialTimeout("tcp", boreServer, 10*time.Second)
	if err != nil {
		return nil, err
	}
	return &boreConn{Conn: c, r: bufio.NewReader(c)}, nil
}

func (c *boreConn) send(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = c.Conn.Write(append(data, 0))
	return err
}

func (c *boreConn) recvServerMsg() (serverMsg, error) {
	data, err := c.r.ReadBytes(0)
	if err != nil {
		return serverMsg{}, err
	}
	return decodeServerMsg(data[:len(data)-1])
}

type serverMsg struct {
	Hello      *uint16
	Connection *string
	Error      *string
	Challenge  *string
	Heartbeat  bool
}

func decodeServerMsg(data []byte) (serverMsg, error) {
	var s string
	if json.Unmarshal(data, &s) == nil {
		if s == "Heartbeat" {
			return serverMsg{Heartbeat: true}, nil
		}
		return serverMsg{}, fmt.Errorf("unknown string message: %q", s)
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return serverMsg{}, fmt.Errorf("decode server message: %w", err)
	}

	var msg serverMsg
	if raw, ok := obj["Hello"]; ok {
		var port uint16
		if err := json.Unmarshal(raw, &port); err != nil {
			return serverMsg{}, err
		}
		msg.Hello = &port
	} else if raw, ok := obj["Connection"]; ok {
		var uuid string
		if err := json.Unmarshal(raw, &uuid); err != nil {
			return serverMsg{}, err
		}
		msg.Connection = &uuid
	} else if raw, ok := obj["Error"]; ok {
		var errMsg string
		if err := json.Unmarshal(raw, &errMsg); err != nil {
			return serverMsg{}, err
		}
		msg.Error = &errMsg
	} else if raw, ok := obj["Challenge"]; ok {
		var uuid string
		if err := json.Unmarshal(raw, &uuid); err != nil {
			return serverMsg{}, err
		}
		msg.Challenge = &uuid
	} else {
		return serverMsg{}, fmt.Errorf("unknown message: %s", data)
	}

	return msg, nil
}

// Start opens a tunnel on bore.pub for localPort and returns the public
// address (e.g. "bore.pub:49152"). The tunnel runs until ctx is cancelled.
func Start(ctx context.Context, localPort int) (string, error) {
	c, err := dial()
	if err != nil {
		return "", fmt.Errorf("connect to bore.pub: %w", err)
	}

	if err := c.send(map[string]any{"Hello": 0}); err != nil {
		c.Close()
		return "", fmt.Errorf("send hello: %w", err)
	}

	msg, err := c.recvServerMsg()
	if err != nil {
		c.Close()
		return "", fmt.Errorf("receive hello: %w", err)
	}
	if msg.Challenge != nil {
		c.Close()
		return "", fmt.Errorf("bore.pub requires authentication")
	}
	if msg.Error != nil {
		c.Close()
		return "", fmt.Errorf("bore.pub: %s", *msg.Error)
	}
	if msg.Hello == nil {
		c.Close()
		return "", fmt.Errorf("expected hello response from bore.pub")
	}

	publicAddr := fmt.Sprintf("bore.pub:%d", *msg.Hello)
	go runControlLoop(ctx, c, localPort)
	return publicAddr, nil
}

func runControlLoop(ctx context.Context, c *boreConn, localPort int) {
	defer c.Close()

	msgCh := make(chan serverMsg, 8)
	errCh := make(chan error, 1)

	go func() {
		for {
			msg, err := c.recvServerMsg()
			if err != nil {
				errCh <- err
				return
			}
			msgCh <- msg
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-errCh:
			return
		case msg := <-msgCh:
			if msg.Heartbeat {
				continue
			}
			if msg.Connection != nil {
				go forwardConnection(*msg.Connection, localPort)
			}
		}
	}
}

func forwardConnection(uuid string, localPort int) {
	c, err := dial()
	if err != nil {
		return
	}

	if err := c.send(map[string]any{"Accept": uuid}); err != nil {
		c.Close()
		return
	}

	localConn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", localPort), 5*time.Second)
	if err != nil {
		c.Close()
		return
	}

	if n := c.r.Buffered(); n > 0 {
		buf := make([]byte, n)
		c.r.Read(buf)
		localConn.Write(buf)
	}

	pipe(c.Conn, localConn)
}

func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() {
		defer func() { done <- struct{}{} }()
		io.Copy(a, b)
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		io.Copy(b, a)
	}()
	<-done
	a.Close()
	b.Close()
}
