package transfer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/JonathanInTheClouds/goxfer/internal/utils"
	"github.com/pkg/sftp"
	"github.com/schollz/progressbar/v3"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/semaphore"
	"golang.org/x/term"
)

// SFTPTransfer handles file or directory transfer logic with parallel support, passphrase-protected keys, and retries for checksum mismatches
func SFTPTransfer(username, password, host, port, keyPath, srcPath, destDir string, maxParallel, maxRetries int) error {

	var authMethod ssh.AuthMethod

	if keyPath != "" {
		// Read the private key file
		key, err := os.ReadFile(keyPath)
		if err != nil {
			return fmt.Errorf("unable to read SSH private key: %v", err)
		}

		// If the private key is passphrase protected, prompt for passphrase
		fmt.Print("Enter passphrase for SSH key: ")
		bytePassphrase, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return fmt.Errorf("failed to read passphrase: %v", err)
		}
		fmt.Println()

		// Parse the private key using the passphrase
		signer, err := ssh.ParsePrivateKeyWithPassphrase(key, bytePassphrase)
		if err != nil {
			return fmt.Errorf("unable to parse private key: %v", err)
		}

		authMethod = ssh.PublicKeys(signer)
	} else {
		// Fallback to password-based authentication
		authMethod = ssh.Password(password)
	}

	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			authMethod,
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	// Establish SSH connection
	conn, err := ssh.Dial("tcp", host+":"+port, config)
	if err != nil {
		return fmt.Errorf("failed to dial: %v", err)
	}
	defer conn.Close()

	// Start an SFTP session
	client, err := sftp.NewClient(conn)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %v", err)
	}
	defer client.Close()

	// Semaphore to limit concurrent transfers
	sem := semaphore.NewWeighted(int64(maxParallel))
	var wg sync.WaitGroup

	// Walk through the source path for file transfers
	err = filepath.Walk(srcPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error accessing path %s: %v", path, err)
		}

		// Compute relative path for the destination
		relativePath, err := filepath.Rel(srcPath, path)
		if err != nil {
			return fmt.Errorf("failed to compute relative path: %v", err)
		}

		remotePath := filepath.Join(destDir, relativePath)

		if info.IsDir() {
			// Create directories directly (without concurrency)
			fmt.Printf("Creating directory: %s\n", remotePath)
			if err := client.MkdirAll(remotePath); err != nil {
				return fmt.Errorf("failed to create remote directory: %v", err)
			}
		} else {
			wg.Add(1)
			go func(path, remotePath string, info os.FileInfo) {
				defer wg.Done()

				// Retry loop for checksum mismatches
				for attempt := 1; attempt <= maxRetries+1; attempt++ {
					if attempt > 1 {
						fmt.Printf("Retrying transfer of %s (Attempt %d of %d)...\n", path, attempt, maxRetries)
					}

					// Use context.Background() instead of nil
					if err := sem.Acquire(context.Background(), 1); err != nil {
						fmt.Printf("Failed to acquire semaphore for file %s: %v\n", path, err)
						return
					}
					defer sem.Release(1)

					fmt.Printf("Transferring file: %s to %s\n", path, remotePath)

					// Calculate the checksum of the local file before transfer
					localChecksum, err := utils.CalculateLocalFileChecksum(path)
					if err != nil {
						fmt.Printf("Failed to calculate checksum for local file %s: %v\n", path, err)
						return
					}

					// Open the local file for reading
					srcFile, err := os.Open(path)
					if err != nil {
						fmt.Printf("Failed to open local source file %s: %v\n", path, err)
						return
					}
					defer srcFile.Close()

					bar := progressbar.DefaultBytes(info.Size(), "Transferring")

					// Create the destination file on the remote server
					dstFile, err := client.Create(remotePath)
					if err != nil {
						fmt.Printf("Failed to create destination file on remote server %s: %v\n", remotePath, err)
						return
					}
					defer dstFile.Close()

					// Copy the file to the remote server with progress tracking
					_, err = io.Copy(io.MultiWriter(dstFile, bar), srcFile)
					if err != nil {
						fmt.Printf("Failed to copy file to remote server %s: %v\n", remotePath, err)
						return
					}

					// Calculate the checksum of the remote file after the transfer
					remoteChecksum, err := utils.CalculateRemoteFileChecksum(client, remotePath)
					if err != nil {
						fmt.Printf("Failed to calculate checksum for remote file %s: %v\n", remotePath, err)
						return
					}

					// Compare the checksums
					if localChecksum == remoteChecksum {
						fmt.Printf("Successfully transferred: %s (checksum verified)\n", path)
						return
					} else {
						fmt.Printf("Checksum mismatch for file %s. Local: %s, Remote: %s\n", path, localChecksum, remoteChecksum)
					}

					// If this was the last attempt, log failure
					if attempt == maxRetries+1 {
						fmt.Printf("Failed to transfer %s after %d attempts\n", path, maxRetries)
					}
				}
			}(path, remotePath, info)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("error walking through files: %v", err)
	}

	wg.Wait() // Wait for all transfers to complete
	fmt.Printf("All transfers completed.\n")
	return nil
}
