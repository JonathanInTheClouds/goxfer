package transfer

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
)

// SCPTransfer handles file transfer using the SCP protocol and creates the destination directory if needed
func SCPTransfer(username, password, host, port, keyPath, srcPath, destDir string, createDestDir bool) error {
	// If createDestDir is true, attempt to create the directory on the remote server
	if createDestDir {
		fmt.Printf("Creating remote directory: %s\n", destDir)
		if err := createRemoteDir(username, host, port, keyPath, destDir); err != nil {
			return fmt.Errorf("failed to create remote directory: %v", err)
		}
	}

	// Check if the destination is a directory
	if !isRemotePathAFile(destDir) {
		destDir = filepath.Join(destDir, filepath.Base(srcPath))
	}

	// Build the SCP command
	scpCmd := []string{
		"-P", port,
	}

	// Add the private key if available
	if keyPath != "" {
		scpCmd = append(scpCmd, "-i", keyPath)
	}

	// Source path and destination (with username@host)
	scpCmd = append(scpCmd, srcPath, fmt.Sprintf("%s@%s:%s", username, host, destDir))

	// Execute the SCP command and capture stderr
	var stderr bytes.Buffer
	cmd := exec.Command("scp", scpCmd...)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error running scp command: %v, details: %s", err, stderr.String())
	}

	fmt.Printf("Successfully transferred file(s) using SCP from %s to %s@%s:%s\n", srcPath, username, host, destDir)
	return nil
}

// createRemoteDir creates the specified directory on the remote server using SSH
func createRemoteDir(username, host, port, keyPath, destDir string) error {
	sshCmd := []string{
		"-p", port,
	}

	if keyPath != "" {
		sshCmd = append(sshCmd, "-i", keyPath)
	}

	// Command to create the directory if it doesn't exist
	sshCmd = append(sshCmd, fmt.Sprintf("%s@%s", username, host), fmt.Sprintf("mkdir -p %s", destDir))

	// Execute the SSH command
	var stderr bytes.Buffer
	cmd := exec.Command("ssh", sshCmd...)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error running ssh command to create directory: %v, details: %s", err, stderr.String())
	}

	fmt.Printf("Remote directory created: %s\n", destDir)
	return nil
}

// isRemotePathAFile checks if the destination path is likely to be a directory or a file
func isRemotePathAFile(dest string) bool {
	// Simple heuristic: if the path ends with a slash, it's likely a directory
	return filepath.Ext(dest) != ""
}
