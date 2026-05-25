package transfer

import (
	"crypto/tls"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jlaffaye/ftp"
	"github.com/schollz/progressbar/v3"
)

// FTPSTransfer handles file or directory transfer over explicit FTPS (FTP with TLS)
func FTPSTransfer(username, password, host, port, srcPath, destDir string, maxRetries int, insecure bool) error {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: insecure,
		ServerName:         host,
	}

	conn, err := ftp.Dial(host+":"+port, ftp.DialWithExplicitTLS(tlsConfig))
	if err != nil {
		return fmt.Errorf("failed to connect to %s:%s: %v", host, port, err)
	}
	defer conn.Quit()

	if err := conn.Login(username, password); err != nil {
		return fmt.Errorf("failed to login as %s: %v", username, err)
	}

	return filepath.Walk(srcPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error accessing path %s: %v", path, err)
		}

		relativePath, err := filepath.Rel(srcPath, path)
		if err != nil {
			return fmt.Errorf("failed to compute relative path: %v", err)
		}

		remotePath := filepath.Join(destDir, relativePath)
		if relativePath == "." {
			remotePath = filepath.Join(destDir, filepath.Base(srcPath))
		}

		if info.IsDir() {
			fmt.Printf("Creating directory: %s\n", remotePath)
			_ = conn.MakeDir(remotePath)
			return nil
		}

		parentDir := filepath.Dir(remotePath)
		_ = conn.MakeDir(parentDir)

		return transferFileWithRetry(conn, path, remotePath, info, maxRetries)
	})
}

func transferFileWithRetry(conn *ftp.ServerConn, localPath, remotePath string, info os.FileInfo, maxRetries int) error {
	for attempt := 1; attempt <= maxRetries+1; attempt++ {
		if attempt > 1 {
			fmt.Printf("Retrying transfer of %s (attempt %d of %d)...\n", localPath, attempt, maxRetries+1)
		}

		fmt.Printf("Transferring file: %s to %s\n", localPath, remotePath)

		srcFile, err := os.Open(localPath)
		if err != nil {
			return fmt.Errorf("failed to open local file %s: %v", localPath, err)
		}

		bar := progressbar.DefaultBytes(info.Size(), "Transferring")
		err = conn.Stor(remotePath, io.TeeReader(srcFile, bar))
		srcFile.Close()

		if err != nil {
			if attempt == maxRetries+1 {
				return fmt.Errorf("failed to transfer %s after %d attempts: %v", localPath, maxRetries+1, err)
			}
			fmt.Printf("Transfer failed for %s: %v\n", localPath, err)
			continue
		}

		remoteSize, err := conn.FileSize(remotePath)
		if err != nil {
			fmt.Printf("Warning: could not verify size of %s: %v\n", remotePath, err)
			return nil
		}

		if remoteSize != info.Size() {
			fmt.Printf("Size mismatch for %s: local=%d remote=%d\n", localPath, info.Size(), remoteSize)
			if attempt == maxRetries+1 {
				return fmt.Errorf("failed to verify %s after %d attempts", localPath, maxRetries+1)
			}
			continue
		}

		fmt.Printf("Successfully transferred: %s (size verified)\n", localPath)
		return nil
	}
	return nil
}
