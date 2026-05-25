package tunnel

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func startTestRelay(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go Serve(l)
	t.Cleanup(func() { l.Close() })
	return l.Addr().String()
}

func TestRelay_SenderReceiver(t *testing.T) {
	addr := startTestRelay(t)

	senderConn, code, err := ConnectAsSender(addr)
	if err != nil {
		t.Fatalf("ConnectAsSender: %v", err)
	}
	defer senderConn.Close()

	if code == "" {
		t.Fatal("expected non-empty code from relay")
	}

	errCh := make(chan error, 1)
	var receiverConn net.Conn
	go func() {
		var e error
		receiverConn, e = ConnectAsReceiver(addr, code)
		errCh <- e
	}()

	if err := WaitForReceiver(senderConn); err != nil {
		t.Fatalf("WaitForReceiver: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("ConnectAsReceiver: %v", err)
	}
	defer receiverConn.Close()

	// Connections are now bridged; verify data flows sender → receiver
	msg := []byte("hello via relay")
	go func() { senderConn.Write(msg) }()

	buf := make([]byte, len(msg))
	receiverConn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := receiverConn.Read(buf); err != nil {
		t.Fatalf("receiver Read: %v", err)
	}
	if !bytes.Equal(buf, msg) {
		t.Fatalf("got %q, want %q", buf, msg)
	}
}

func TestRelay_BidirectionalData(t *testing.T) {
	addr := startTestRelay(t)

	senderConn, code, err := ConnectAsSender(addr)
	if err != nil {
		t.Fatalf("ConnectAsSender: %v", err)
	}
	defer senderConn.Close()

	errCh := make(chan error, 1)
	var receiverConn net.Conn
	go func() {
		var e error
		receiverConn, e = ConnectAsReceiver(addr, code)
		errCh <- e
	}()

	if err := WaitForReceiver(senderConn); err != nil {
		t.Fatalf("WaitForReceiver: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("ConnectAsReceiver: %v", err)
	}
	defer receiverConn.Close()

	deadline := time.Now().Add(2 * time.Second)
	senderConn.SetDeadline(deadline)
	receiverConn.SetDeadline(deadline)

	// sender → receiver
	senderConn.Write([]byte("ping"))
	buf := make([]byte, 4)
	if _, err := receiverConn.Read(buf); err != nil {
		t.Fatalf("receiver Read: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("sender→receiver: got %q, want %q", buf, "ping")
	}

	// receiver → sender
	receiverConn.Write([]byte("pong"))
	if _, err := senderConn.Read(buf); err != nil {
		t.Fatalf("sender Read: %v", err)
	}
	if string(buf) != "pong" {
		t.Fatalf("receiver→sender: got %q, want %q", buf, "pong")
	}
}

func TestRelay_InvalidCode(t *testing.T) {
	addr := startTestRelay(t)

	_, err := ConnectAsReceiver(addr, "doesnotexist")
	if err == nil {
		t.Fatal("expected error for invalid relay code, got nil")
	}
}

func TestRelay_CodeUnique(t *testing.T) {
	addr := startTestRelay(t)

	conn1, code1, err := ConnectAsSender(addr)
	if err != nil {
		t.Fatalf("ConnectAsSender 1: %v", err)
	}
	defer conn1.Close()

	conn2, code2, err := ConnectAsSender(addr)
	if err != nil {
		t.Fatalf("ConnectAsSender 2: %v", err)
	}
	defer conn2.Close()

	if code1 == code2 {
		t.Fatalf("two senders received the same relay code: %q", code1)
	}
}

func TestRelay_CodeConsumedOnConnect(t *testing.T) {
	addr := startTestRelay(t)

	senderConn, code, err := ConnectAsSender(addr)
	if err != nil {
		t.Fatalf("ConnectAsSender: %v", err)
	}
	defer senderConn.Close()

	errCh := make(chan error, 1)
	var receiverConn net.Conn
	go func() {
		var e error
		receiverConn, e = ConnectAsReceiver(addr, code)
		errCh <- e
	}()

	WaitForReceiver(senderConn)
	if err := <-errCh; err != nil {
		t.Fatalf("first receiver: %v", err)
	}
	defer receiverConn.Close()

	// A second receiver with the same code must fail — code is consumed
	_, err = ConnectAsReceiver(addr, code)
	if err == nil {
		t.Fatal("expected error when reusing a relay code, got nil")
	}
}
