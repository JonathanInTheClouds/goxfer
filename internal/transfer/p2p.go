package transfer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
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
// When resume=true the file ID is deterministic so a retry can pick up where it left off.
func P2PSend(srcPath, relayAddr string, resume bool) error {
	identity, err := crypto.GenerateIdentity()
	if err != nil {
		return fmt.Errorf("generate identity: %w", err)
	}

	var sess *session.SecureSession

	if relayAddr == "" {
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

		printReceiverCommand("goxfer receive", resume, "", publicAddr)
		fmt.Printf("Your fingerprint : %s\n", identity.Fingerprint())
		fmt.Println("\nWaiting for receiver to connect...")

		sess, err = listener.Accept()
		if err != nil {
			return fmt.Errorf("accept connection: %w", err)
		}
	} else {
		conn, code, err := tunnel.ConnectAsSender(relayAddr)
		if err != nil {
			return fmt.Errorf("connect to relay: %w", err)
		}

		printReceiverCommand("goxfer receive", resume, code, relayAddr)
		fmt.Printf("Your fingerprint : %s\n", identity.Fingerprint())
		fmt.Println("\nWaiting for receiver to connect...")

		if err := tunnel.WaitForReceiver(conn); err != nil {
			return fmt.Errorf("wait for receiver: %w", err)
		}

		sess, err = session.NewSession(conn, identity, false)
		if err != nil {
			return fmt.Errorf("establish session: %w", err)
		}
	}
	defer sess.Close()

	fmt.Print("Receiver connected!\n\n")

	info, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	if info.IsDir() {
		return sendDirectory(sess, srcPath)
	}
	return sendSingleFile(sess, srcPath, info, resume)
}

// P2PReceive connects to a sender and downloads files into destDir.
// For bore.pub: addr=bore.pub:NNNNN, code="".
// For self-hosted relay: addr=relay:port, code=<code>.
func P2PReceive(addr, destDir, code string, resume bool) error {
	identity, err := crypto.GenerateIdentity()
	if err != nil {
		return fmt.Errorf("generate identity: %w", err)
	}

	var sess *session.SecureSession

	fmt.Printf("Connecting to sender at %s...\n", addr)

	if code == "" {
		sess, err = session.Dial(addr, identity)
		if err != nil {
			return fmt.Errorf("connect to sender: %w", err)
		}
	} else {
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

	fmt.Printf("Connected. Your fingerprint: %s\n\n", identity.Fingerprint())

	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	return receiveFiles(sess, destDir, resume)
}

func sendSingleFile(sess *session.SecureSession, path string, info os.FileInfo, resume bool) error {
	localChecksum, err := utils.CalculateLocalFileChecksum(path)
	if err != nil {
		return fmt.Errorf("checksum local file: %w", err)
	}

	var fileID string
	if resume {
		fileID = deterministicFileID(path, info.Size())
	} else {
		fileID, err = randomFileID()
		if err != nil {
			return err
		}
	}

	if err := sess.SendMessage(protocol.Message{
		Type:   protocol.MessageTypeFileStart,
		FileID: fileID,
		Name:   filepath.Base(path),
		Size:   info.Size(),
		Resume: resume,
	}); err != nil {
		return err
	}

	// When resume is on, wait for the receiver's ack before sending data.
	var startOffset int64
	var startIndex int
	if resume {
		ack, err := sess.ReceiveMessage()
		if err != nil {
			return fmt.Errorf("receive resume ack: %w", err)
		}
		if ack.Type == protocol.MessageTypeFileResume {
			startOffset = ack.Offset
			startIndex = int(startOffset / protocol.FileChunkSize)
		}
		// MessageTypeReady means start from zero — defaults are already 0
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	if startOffset > 0 {
		if _, err := f.Seek(startOffset, io.SeekStart); err != nil {
			return fmt.Errorf("seek to resume offset: %w", err)
		}
		fmt.Printf("Resuming from %s / %s\n", formatBytes(startOffset), formatBytes(info.Size()))
	}

	fmt.Printf("Sending  %s  (%s)\n", filepath.Base(path), formatBytes(info.Size()))
	bar := newBar(info.Size())
	bar.Set64(startOffset)
	if err := sendChunks(sess, fileID, io.TeeReader(f, bar), startIndex); err != nil {
		return err
	}
	bar.Finish()

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

	ack, err := sess.ReceiveMessage()
	if err != nil {
		return fmt.Errorf("receive checksum ack: %w", err)
	}
	if ack.Type != protocol.MessageTypeFileChecksum || ack.Checksum != localChecksum {
		return fmt.Errorf("checksum mismatch confirmed by receiver")
	}

	fmt.Printf("\n✓  Sent successfully — checksum verified\n")
	return nil
}

// sendDirectory streams srcPath as a tar.gz. Resume is not supported for directories
// because the archive is generated on the fly and cannot be seeked.
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
		Size:   -1,
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

	fmt.Printf("Sending  %s/  (streaming)\n", filepath.Base(srcPath))
	bar := newBar(-1)
	if err := sendChunks(sess, fileID, io.TeeReader(io.TeeReader(pr, hasher), bar), 0); err != nil {
		return err
	}
	bar.Finish()

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

	fmt.Printf("\n✓  Sent successfully — checksum verified\n")
	return nil
}

func sendChunks(sess *session.SecureSession, fileID string, r io.Reader, startIndex int) error {
	buf := make([]byte, protocol.FileChunkSize)
	index := startIndex
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

func receiveFiles(sess *session.SecureSession, destDir string, resume bool) error {
	for {
		msg, err := sess.ReceiveMessage()
		if err != nil {
			return nil
		}

		if msg.Type != protocol.MessageTypeFileStart {
			return fmt.Errorf("expected file_start, got %q", msg.Type)
		}

		if err := receiveOneFile(sess, destDir, msg, resume); err != nil {
			return err
		}
	}
}

func receiveOneFile(sess *session.SecureSession, destDir string, start protocol.Message, resume bool) error {
	isArchive := strings.HasSuffix(start.Name, ".tar.gz")

	// Attempt to resume only for regular files where the sender also opted in.
	var state *resumeState
	if resume && start.Resume && !isArchive {
		state = loadResumeState(destDir, start.FileID)
	}

	var (
		tmp       *os.File
		hasher    hash.Hash
		nextIndex int
	)

	if state != nil {
		// Re-open the existing temp file and restore state.
		existing, err := os.OpenFile(state.TempPath, os.O_RDWR, 0o600)
		if err != nil {
			// Temp file gone — fall back to fresh start.
			state = nil
		} else {
			resumeOffset := state.byteOffset()

			// Truncate to a safe chunk boundary (discards any partial last write).
			if err := existing.Truncate(resumeOffset); err != nil {
				existing.Close()
				state = nil
			} else if _, err := existing.Seek(resumeOffset, io.SeekStart); err != nil {
				existing.Close()
				state = nil
			} else {
				tmp = existing
				nextIndex = state.NextIndex
				hasher = sha256.New()
				fmt.Printf("Restoring checksum for %s (%s already received)...\n",
					start.Name, formatBytes(resumeOffset))
				if err := rehashFile(state.TempPath, resumeOffset, hasher); err != nil {
					tmp.Close()
					state = nil
					nextIndex = 0
					hasher = nil
				}
			}
		}
	}

	if state == nil {
		// Fresh start.
		newTmp, err := os.CreateTemp("", "goxfer-recv-*")
		if err != nil {
			return fmt.Errorf("create temp file: %w", err)
		}
		tmp = newTmp
		hasher = sha256.New()
		nextIndex = 0

		if resume && start.Resume && !isArchive {
			state = &resumeState{
				FileID:    start.FileID,
				Name:      start.Name,
				Size:      start.Size,
				NextIndex: 0,
				TempPath:  tmp.Name(),
			}
			saveResumeState(destDir, state)
		}
	}

	tmpPath := tmp.Name()
	defer func() { os.Remove(tmpPath) }()

	// Ack the sender when resume handshake is active.
	if start.Resume {
		if nextIndex > 0 {
			resumeOffset := int64(nextIndex) * protocol.FileChunkSize
			if err := sess.SendMessage(protocol.Message{
				Type:   protocol.MessageTypeFileResume,
				FileID: start.FileID,
				Offset: resumeOffset,
			}); err != nil {
				tmp.Close()
				return fmt.Errorf("send file_resume: %w", err)
			}
			fmt.Printf("Resuming from %s\n", formatBytes(resumeOffset))
		} else {
			if err := sess.SendMessage(protocol.Message{Type: protocol.MessageTypeReady}); err != nil {
				tmp.Close()
				return fmt.Errorf("send ready: %w", err)
			}
		}
	}

	label := start.Name
	if isArchive {
		label = strings.TrimSuffix(start.Name, ".tar.gz") + "/"
	}
	if start.Size > 0 {
		fmt.Printf("Receiving  %s  (%s)\n", label, formatBytes(start.Size))
	} else {
		fmt.Printf("Receiving  %s  (streaming)\n", label)
	}

	bar := newBar(start.Size)
	bar.Set64(int64(nextIndex) * protocol.FileChunkSize)

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

			if state != nil {
				state.NextIndex = nextIndex
				saveResumeState(destDir, state)
			}

		case protocol.MessageTypeFileComplete:
			tmp.Close()
			if msg.FileID != start.FileID {
				return fmt.Errorf("unexpected file_id in file_complete")
			}

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

			if err := sess.SendMessage(protocol.Message{
				Type:     protocol.MessageTypeFileChecksum,
				FileID:   start.FileID,
				Checksum: localChecksum,
			}); err != nil {
				return fmt.Errorf("send checksum ack: %w", err)
			}

			if state != nil {
				deleteResumeState(destDir, start.FileID)
			}

			bar.Finish()
			fmt.Println()

			if isArchive {
				fmt.Printf("Extracting %s...\n", start.Name)
				if err := extractTarGz(tmpPath, destDir); err != nil {
					return fmt.Errorf("extract archive: %w", err)
				}
				fmt.Printf("✓  Saved to %s — checksum verified\n", destDir)
			} else {
				destPath := filepath.Join(destDir, filepath.Base(start.Name))
				if err := os.Rename(tmpPath, destPath); err != nil {
					if err2 := copyFile(tmpPath, destPath); err2 != nil {
						return fmt.Errorf("save file: %w", err2)
					}
				}
				fmt.Printf("✓  Saved to %s — checksum verified\n", destPath)
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

// resumeState is persisted alongside a partial download so the receiver can
// hand the correct byte offset back to the sender on reconnect.
type resumeState struct {
	FileID    string `json:"file_id"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	NextIndex int    `json:"next_index"`
	TempPath  string `json:"temp_path"`
}

func (s *resumeState) byteOffset() int64 {
	return int64(s.NextIndex) * protocol.FileChunkSize
}

func resumeStatePath(destDir, fileID string) string {
	return filepath.Join(destDir, ".goxfer-"+fileID+".state")
}

func loadResumeState(destDir, fileID string) *resumeState {
	data, err := os.ReadFile(resumeStatePath(destDir, fileID))
	if err != nil {
		return nil
	}
	var state resumeState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil
	}
	return &state
}

func saveResumeState(destDir string, state *resumeState) {
	data, _ := json.Marshal(state)
	os.WriteFile(resumeStatePath(destDir, state.FileID), data, 0o600)
}

func deleteResumeState(destDir, fileID string) {
	os.Remove(resumeStatePath(destDir, fileID))
}

// deterministicFileID derives a stable file ID from path and size so retries
// produce the same ID and the receiver can match a partial download.
func deterministicFileID(path string, size int64) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%d", path, size)
	return hex.EncodeToString(h.Sum(nil))[:24]
}

// rehashFile feeds the first n bytes of path into h to restore a mid-transfer
// SHA256 state when resuming a download.
func rehashFile(path string, n int64, h hash.Hash) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.CopyN(h, f, n)
	if err == io.EOF {
		return nil
	}
	return err
}

func randomFileID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate file id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

// printReceiverCommand prints a clearly bordered block with the command
// the receiver needs to run. Flags are placed before positional arguments
// so Go's flag parser picks them up correctly.
func printReceiverCommand(base string, resume bool, code, addr string) {
	cmd := base
	if code != "" {
		cmd += " --code=" + code
	}
	if resume {
		cmd += " --resume"
	}
	cmd += " " + addr + " <dest-dir>"
	border := strings.Repeat("─", len(cmd)+4)
	fmt.Printf("\n┌%s┐\n│  %s  │\n└%s┘\n\n", border, cmd, border)
	fmt.Println("  Run the command above on the receiving machine.")
}

// newBar returns a progress bar suitable for file transfer output.
func newBar(size int64) *progressbar.ProgressBar {
	return progressbar.NewOptions64(size,
		progressbar.OptionSetWidth(40),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetItsString("B"),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionSetElapsedTime(true),
		progressbar.OptionShowElapsedTimeOnFinish(),
		progressbar.OptionUseIECUnits(true),
		progressbar.OptionFullWidth(),
	)
}

// formatBytes returns a human-readable byte size string.
func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
