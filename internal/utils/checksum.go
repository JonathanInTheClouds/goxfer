package utils

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"

	"github.com/pkg/sftp"
)

// Calculate the SHA256 checksum for a local file
func CalculateLocalFileChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open local file for checksum: %v", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to calculate checksum for local file: %v", err)
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// Calculate the SHA256 checksum for a remote file
func CalculateRemoteFileChecksum(client *sftp.Client, remotePath string) (string, error) {
	file, err := client.Open(remotePath)
	if err != nil {
		return "", fmt.Errorf("failed to open remote file for checksum: %v", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to calculate checksum for remote file: %v", err)
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
