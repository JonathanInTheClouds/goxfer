package transfer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/sftp"
	"github.com/schollz/progressbar/v3"
	"golang.org/x/crypto/ssh"
)

// SFTPTransfer handles file or directory transfer logic
func SFTPTransfer(username, password, host, port, srcPath, destDir string) error {

	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
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

	// Walk the source path (this works for both files and directories)
	err = filepath.Walk(srcPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error accessing path %s: %v", path, err)
		}

		// Relative path for file transfer destination
		relativePath, err := filepath.Rel(srcPath, path)
		if err != nil {
			return fmt.Errorf("failed to compute relative path: %v", err)
		}

		// Full destination path on the remote server
		remotePath := filepath.Join(destDir, relativePath)

		if info.IsDir() {
			// If it's a directory, create the directory on the remote server
			fmt.Printf("Creating directory: %s\n", remotePath)
			if err := client.MkdirAll(remotePath); err != nil {
				return fmt.Errorf("failed to create remote directory: %v", err)
			}
		} else {
			// If it's a file, transfer it
			fmt.Printf("Transferring file: %s to %s\n", path, remotePath)

			// Open the local file
			srcFile, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open local source file: %v", err)
			}
			defer srcFile.Close()

			// Create a progress bar for the file transfer
			bar := progressbar.DefaultBytes(
				info.Size(),
				"Transferring",
			)

			// Create the destination file on the remote server
			dstFile, err := client.Create(remotePath)
			if err != nil {
				return fmt.Errorf("failed to create destination file on remote server: %v", err)
			}
			defer dstFile.Close()

			// Copy the file to the remote server with progress tracking
			_, err = io.Copy(io.MultiWriter(dstFile, bar), srcFile)
			if err != nil {
				return fmt.Errorf("failed to copy file to remote server: %v", err)
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("error walking through files: %v", err)
	}

	fmt.Printf("\nSuccessfully transferred %s to %s on %s\n", srcPath, destDir, host)
	return nil
}
