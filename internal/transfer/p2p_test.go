package transfer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/JonathanInTheClouds/goxfer/internal/crypto"
	"github.com/JonathanInTheClouds/goxfer/internal/session"
)

// makePair returns a (sender, receiver) SecureSession pair over net.Pipe().
// Sender = Noise responder, receiver = Noise initiator, matching p2p.go convention.
func makePair(t *testing.T) (senderSess, receiverSess *session.SecureSession) {
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
		sess *session.SecureSession
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		s, e := session.NewSession(connA, senderID, false) // sender = responder
		ch <- result{s, e}
	}()

	recvSess, err := session.NewSession(connB, receiverID, true) // receiver = initiator
	if err != nil {
		connA.Close()
		connB.Close()
		t.Fatalf("receiver NewSession: %v", err)
	}

	r := <-ch
	if r.err != nil {
		recvSess.Close()
		t.Fatalf("sender NewSession: %v", r.err)
	}

	return r.sess, recvSess
}

func TestP2P_SingleFile(t *testing.T) {
	senderSess, receiverSess := makePair(t)

	content := []byte("hello goxfer p2p transfer test payload")
	srcFile, err := os.CreateTemp("", "goxfer-send-*.txt")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	srcFile.Write(content)
	srcFile.Close()
	srcPath := srcFile.Name()
	defer os.Remove(srcPath)

	destDir := t.TempDir()

	sendErr := make(chan error, 1)
	recvErr := make(chan error, 1)

	go func() {
		info, err := os.Stat(srcPath)
		if err != nil {
			sendErr <- err
			return
		}
		err = sendSingleFile(senderSess, srcPath, info)
		senderSess.Close() // signal EOF to receiver
		sendErr <- err
	}()
	go func() {
		recvErr <- receiveFiles(receiverSess, destDir)
	}()

	if err := <-sendErr; err != nil {
		t.Fatalf("send error: %v", err)
	}
	if err := <-recvErr; err != nil {
		t.Fatalf("receive error: %v", err)
	}

	destPath := filepath.Join(destDir, filepath.Base(srcPath))
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read received file: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("content mismatch: got %q, want %q", got, content)
	}
}

func TestP2P_ChecksumVerification(t *testing.T) {
	senderSess, receiverSess := makePair(t)

	// Fill with patterned data across multiple chunks
	content := make([]byte, 3*32*1024+500) // slightly over 3 chunks
	for i := range content {
		content[i] = byte(i % 251)
	}

	srcFile, err := os.CreateTemp("", "goxfer-chk-*.bin")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	srcFile.Write(content)
	srcFile.Close()
	srcPath := srcFile.Name()
	defer os.Remove(srcPath)

	expectedSum := sha256.Sum256(content)
	expectedChecksum := hex.EncodeToString(expectedSum[:])

	destDir := t.TempDir()

	sendErr := make(chan error, 1)
	recvErr := make(chan error, 1)

	go func() {
		info, _ := os.Stat(srcPath)
		err := sendSingleFile(senderSess, srcPath, info)
		senderSess.Close()
		sendErr <- err
	}()
	go func() {
		recvErr <- receiveFiles(receiverSess, destDir)
	}()

	if err := <-sendErr; err != nil {
		t.Fatalf("send error: %v", err)
	}
	if err := <-recvErr; err != nil {
		t.Fatalf("receive error: %v", err)
	}

	destPath := filepath.Join(destDir, filepath.Base(srcPath))
	received, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read received file: %v", err)
	}

	gotSum := sha256.Sum256(received)
	gotChecksum := hex.EncodeToString(gotSum[:])
	if gotChecksum != expectedChecksum {
		t.Fatalf("checksum mismatch: got %s, want %s", gotChecksum, expectedChecksum)
	}
}

func TestP2P_Directory(t *testing.T) {
	senderSess, receiverSess := makePair(t)

	srcDir := t.TempDir()
	files := map[string]string{
		"readme.txt":      "top-level readme",
		"sub/data.bin":    "nested binary data",
		"sub/deep/a.txt":  "deeply nested file",
	}
	for name, content := range files {
		path := filepath.Join(srcDir, name)
		os.MkdirAll(filepath.Dir(path), 0o750)
		os.WriteFile(path, []byte(content), 0o644)
	}

	destDir := t.TempDir()

	sendErr := make(chan error, 1)
	recvErr := make(chan error, 1)

	go func() {
		err := sendDirectory(senderSess, srcDir)
		senderSess.Close()
		sendErr <- err
	}()
	go func() {
		recvErr <- receiveFiles(receiverSess, destDir)
	}()

	if err := <-sendErr; err != nil {
		t.Fatalf("send error: %v", err)
	}
	if err := <-recvErr; err != nil {
		t.Fatalf("receive error: %v", err)
	}

	base := filepath.Base(srcDir)
	for name, wantContent := range files {
		destPath := filepath.Join(destDir, base, name)
		got, err := os.ReadFile(destPath)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(got) != wantContent {
			t.Fatalf("%s: got %q, want %q", name, got, wantContent)
		}
	}
}

func TestExtractTarGz_ZipSlip(t *testing.T) {
	tests := []struct {
		name    string
		tarName string
	}{
		{"path traversal dotdot", "../evil.txt"},
		{"absolute path", "/etc/evil.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			gw := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gw)

			hdr := &tar.Header{
				Name:     tt.tarName,
				Typeflag: tar.TypeReg,
				Size:     5,
				Mode:     0644,
			}
			tw.WriteHeader(hdr)
			tw.Write([]byte("evil!"))
			tw.Close()
			gw.Close()

			tmp, err := os.CreateTemp("", "goxfer-zipslip-*")
			if err != nil {
				t.Fatalf("create temp: %v", err)
			}
			tmp.Write(buf.Bytes())
			tmp.Close()
			defer os.Remove(tmp.Name())

			destDir := t.TempDir()
			if err := extractTarGz(tmp.Name(), destDir); err == nil {
				t.Fatalf("expected zip-slip error for %q, got nil", tt.tarName)
			}
		})
	}
}

func TestExtractTarGz_ValidArchive(t *testing.T) {
	files := map[string]string{
		"mydir/a.txt":     "file a",
		"mydir/sub/b.txt": "file b in sub",
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Write dir header
	tw.WriteHeader(&tar.Header{Name: "mydir/", Typeflag: tar.TypeDir, Mode: 0750})
	tw.WriteHeader(&tar.Header{Name: "mydir/sub/", Typeflag: tar.TypeDir, Mode: 0750})

	for name, content := range files {
		tw.WriteHeader(&tar.Header{
			Name:     name,
			Typeflag: tar.TypeReg,
			Size:     int64(len(content)),
			Mode:     0644,
		})
		tw.Write([]byte(content))
	}
	tw.Close()
	gw.Close()

	tmp, err := os.CreateTemp("", "goxfer-tarvalid-*")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	tmp.Write(buf.Bytes())
	tmp.Close()
	defer os.Remove(tmp.Name())

	destDir := t.TempDir()
	if err := extractTarGz(tmp.Name(), destDir); err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	for name, wantContent := range files {
		got, err := os.ReadFile(filepath.Join(destDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(got) != wantContent {
			t.Fatalf("%s: got %q, want %q", name, got, wantContent)
		}
	}
}
