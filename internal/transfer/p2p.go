package transfer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/JonathanInTheClouds/goxfer/internal/crypto"
	"github.com/JonathanInTheClouds/goxfer/internal/protocol"
	"github.com/JonathanInTheClouds/goxfer/internal/session"
	"github.com/JonathanInTheClouds/goxfer/internal/tunnel"
	"github.com/JonathanInTheClouds/goxfer/internal/utils"
	"github.com/schollz/progressbar/v3"
)

// P2PSend sends srcPath to a peer. relayAddr="" uses bore.pub; otherwise uses self-hosted relay.
func P2PSend(srcPath, relayAddr string) error {
	identity, err := crypto.GenerateIdentity()
	if err != nil {
		return fmt.Errorf("generate identity: %w", err)
	}

	var sess *session.SecureSession

	if relayAddr == "" {
		// bore.pub mode: bind local port, tunnel via bore.pub
		listener, localPort, err := session.Bind(":0", identity)
		if err != nil {
			return fmt.Errorf("bind local listener: %w", err)
		}
		defer listener.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		publicAddr, err := tunnel.Start(ctx, localPort)
		if err != nil {
			return fmt.Errorf("start bore.pub tunnel: %w", err)
		}

		fmt.Printf("Share this with the receiver:\n\n  goxfer receive %s <destDir>\n\n", publicAddr)
		fmt.Println("Waiting for receiver to connect...")

		sess, err = listener.Accept()
		if err != nil {
			return fmt.Errorf("accept connection: %w", err)
		}
	} else {
		// Self-hosted relay mode
		conn, code, err := tunnel.ConnectAsSender(relayAddr)
		if err != nil {
			return fmt.Errorf("connect to relay: %w", err)
		}

		fmt.Printf("Share this with the receiver:\n\n  goxfer receive %s <destDir> --code=%s\n\n", relayAddr, code)
		fmt.Println("Waiting for receiver to connect...")

		if err := tunnel.WaitForReceiver(conn); err != nil {
			return fmt.Errorf("wait for receiver: %w", err)
		}

		sess, err = session.NewSession(conn, identity, false)
		if err != nil {
			return fmt.Errorf("establish session: %w", err)
		}
	}
	defer sess.Close()

	fmt.Printf("Peer connected. Sender fingerprint: %s\n", identity.Fingerprint())

	info, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	if info.IsDir() {
		return sendDirectory(sess, srcPath)
	}
	return sendSingleFile(sess, srcPath, info)
}

// P2PReceive connects to a sender and downloads files into destDir.
// For bore.pub: addr=bore.pub:NNNNN, code="".
// For self-hosted relay: addr=relay:port, code=<code>.
func P2PReceive(addr, destDir, code string) error {
	identity, err := crypto.GenerateIdentity()
	if err != nil {
		return fmt.Errorf("generate identity: %w", err)
	}

	var sess *session.SecureSession

	if code == "" {
		// bore.pub mode: direct dial
		sess, err = session.Dial(addr, identity)
		if err != nil {
			return fmt.Errorf("connect to sender: %w", err)
		}
	} else {
		// Self-hosted relay mode
		conn, err := tunnel.ConnectAsReceiver(addr, code)
		if err != nil {
			return fmt.Errorf("connect to relay: %w", err)
		}
		sess, err = session.NewSession(conn, identity, true)
		if err != nil {
			return fmt.Errorf("establish session: %w", err)
		}
	}
	defer sess.Close()

	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	return receiveFiles(sess, destDir)
}

func sendSingleFile(sess *session.SecureSession, path string, info os.FileInfo) error {
	localChecksum, err := utils.CalculateLocalFileChecksum(path)
	if err != nil {
		return fmt.Errorf("checksum local file: %w", err)
	}

	fileID, err := randomFileID()
	if err != nil {
		return err
	}

	if err := sess.SendMessage(protocol.Message{
		Type:   protocol.MessageTypeFileStart,
		FileID: fileID,
		Name:   filepath.Base(path),
		Size:   info.Size(),
	}); err != nil {
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	bar := progressbar.DefaultBytes(info.Size(), "Sending")
	if err := sendChunks(sess, fileID, io.TeeReader(f, bar)); err != nil {
		return err
	}

	if err := sess.SendMessage(protocol.Message{
		Type:   protocol.MessageTypeFileComplete,
		FileID: fileID,
	}); err != nil {
		return err
	}

	if err := sess.SendMessage(protocol.Message{
		Type:     protocol.MessageTypeFileChecksum,
		FileID:   fileID,
		Checksum: localChecksum,
	}); err != nil {
		return err
	}

	// Wait for receiver's echo to confirm checksum matched
	ack, err := sess.ReceiveMessage()
	if err != nil {
		return fmt.Errorf("receive checksum ack: %w", err)
	}
	if ack.Type != protocol.MessageTypeFileChecksum || ack.Checksum != localChecksum {
		return fmt.Errorf("checksum mismatch confirmed by receiver")
	}

	fmt.Printf("Successfully sent: %s (checksum verified)\n", path)
	return nil
}

func sendDirectory(sess *session.SecureSession, srcPath string) error {
	fileID, err := randomFileID()
	if err != nil {
		return err
	}

	archiveName := filepath.Base(srcPath) + ".tar.gz"

	if err := sess.SendMessage(protocol.Message{
		Type:   protocol.MessageTypeFileStart,
		FileID: fileID,
		Name:   archiveName,
		Size:   -1, // streaming — size unknown
	}); err != nil {
		return err
	}

	pr, pw := io.Pipe()
	hasher := sha256.New()

	var archiveErr error
	go func() {
		gw := gzip.NewWriter(pw)
		tw := tar.NewWriter(gw)
		archiveErr = filepath.Walk(srcPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(srcPath, path)
			if err != nil {
				return err
			}
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			hdr.Name = filepath.Join(filepath.Base(srcPath), rel)
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if !info.IsDir() {
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				defer f.Close()
				if _, err := io.Copy(tw, f); err != nil {
					return err
				}
			}
			return nil
		})
		tw.Close()
		gw.Close()
		pw.CloseWithError(archiveErr)
	}()

	bar := progressbar.DefaultBytes(-1, "Sending")
	if err := sendChunks(sess, fileID, io.TeeReader(io.TeeReader(pr, hasher), bar)); err != nil {
		return err
	}

	if archiveErr != nil {
		return fmt.Errorf("archive directory: %w", archiveErr)
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))

	if err := sess.SendMessage(protocol.Message{
		Type:   protocol.MessageTypeFileComplete,
		FileID: fileID,
	}); err != nil {
		return err
	}

	if err := sess.SendMessage(protocol.Message{
		Type:     protocol.MessageTypeFileChecksum,
		FileID:   fileID,
		Checksum: checksum,
	}); err != nil {
		return err
	}

	ack, err := sess.ReceiveMessage()
	if err != nil {
		return fmt.Errorf("receive checksum ack: %w", err)
	}
	if ack.Type != protocol.MessageTypeFileChecksum || ack.Checksum != checksum {
		return fmt.Errorf("checksum mismatch confirmed by receiver")
	}

	fmt.Printf("Successfully sent directory: %s (checksum verified)\n", srcPath)
	return nil
}

func sendChunks(sess *session.SecureSession, fileID string, r io.Reader) error {
	buf := make([]byte, protocol.FileChunkSize)
	index := 0
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if err := sess.SendMessage(protocol.Message{
				Type:   protocol.MessageTypeFileChunk,
				FileID: fileID,
				Index:  index,
				Chunk:  chunk,
			}); err != nil {
				return err
			}
			index++
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read source: %w", readErr)
		}
	}
	return nil
}

func receiveFiles(sess *session.SecureSession, destDir string) error {
	for {
		msg, err := sess.ReceiveMessage()
		if err != nil {
			// Connection closed — done
			return nil
		}

		if msg.Type != protocol.MessageTypeFileStart {
			return fmt.Errorf("expected file_start, got %q", msg.Type)
		}

		if err := receiveOneFile(sess, destDir, msg); err != nil {
			return err
		}
	}
}

func receiveOneFile(sess *session.SecureSession, destDir string, start protocol.Message) error {
	tmp, err := os.CreateTemp("", "goxfer-recv-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { os.Remove(tmpPath) }()

	hasher := sha256.New()
	bar := progressbar.DefaultBytes(start.Size, "Receiving")

	var nextIndex int
	for {
		msg, err := sess.ReceiveMessage()
		if err != nil {
			tmp.Close()
			return fmt.Errorf("receive chunk: %w", err)
		}

		switch msg.Type {
		case protocol.MessageTypeFileChunk:
			if msg.FileID != start.FileID {
				tmp.Close()
				return fmt.Errorf("unexpected file_id in chunk")
			}
			if msg.Index != nextIndex {
				tmp.Close()
				return fmt.Errorf("out-of-order chunk: got %d, want %d", msg.Index, nextIndex)
			}
			if _, err := tmp.Write(msg.Chunk); err != nil {
				tmp.Close()
				return fmt.Errorf("write chunk: %w", err)
			}
			hasher.Write(msg.Chunk)
			bar.Add(len(msg.Chunk))
			nextIndex++

		case protocol.MessageTypeFileComplete:
			tmp.Close()
			if msg.FileID != start.FileID {
				return fmt.Errorf("unexpected file_id in file_complete")
			}

			// Read checksum from sender
			checksumMsg, err := sess.ReceiveMessage()
			if err != nil {
				return fmt.Errorf("receive checksum: %w", err)
			}
			if checksumMsg.Type != protocol.MessageTypeFileChecksum {
				return fmt.Errorf("expected file_checksum, got %q", checksumMsg.Type)
			}

			localChecksum := hex.EncodeToString(hasher.Sum(nil))
			if localChecksum != checksumMsg.Checksum {
				return fmt.Errorf("checksum mismatch: got %s, want %s", localChecksum, checksumMsg.Checksum)
			}

			// Echo checksum back to sender as confirmation
			if err := sess.SendMessage(protocol.Message{
				Type:     protocol.MessageTypeFileChecksum,
				FileID:   start.FileID,
				Checksum: localChecksum,
			}); err != nil {
				return fmt.Errorf("send checksum ack: %w", err)
			}

			// Move/extract to final destination
			isArchive := strings.HasSuffix(start.Name, ".tar.gz")
			if isArchive {
				fmt.Printf("\nExtracting %s...\n", start.Name)
				if err := extractTarGz(tmpPath, destDir); err != nil {
					return fmt.Errorf("extract archive: %w", err)
				}
			} else {
				destPath := filepath.Join(destDir, filepath.Base(start.Name))
				if err := os.Rename(tmpPath, destPath); err != nil {
					// Rename across devices may fail — fall back to copy
					if err2 := copyFile(tmpPath, destPath); err2 != nil {
						return fmt.Errorf("save file: %w", err2)
					}
				}
				fmt.Printf("\nSuccessfully received: %s (checksum verified)\n", destPath)
			}
			return nil

		default:
			tmp.Close()
			return fmt.Errorf("unexpected message type %q during file receive", msg.Type)
		}
	}
}

func extractTarGz(srcPath, destDir string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	absDestDir, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Zip-slip protection
		clean := filepath.Clean(hdr.Name)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return fmt.Errorf("rejected unsafe tar entry: %q", hdr.Name)
		}

		target := filepath.Join(absDestDir, clean)
		if !strings.HasPrefix(target, absDestDir+string(filepath.Separator)) {
			return fmt.Errorf("rejected tar entry escaping destination: %q", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode())
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}

	fmt.Printf("Extracted to: %s\n", destDir)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func randomFileID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate file id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}
