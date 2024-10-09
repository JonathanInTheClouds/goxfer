package transfer

import (
	"fmt"
	"io"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SFTPTransfer handles the SFTP file transfer logic
func SFTPTransfer(username, password, host, port, file, dest string) error {
	// Implement your SFTP logic here
	// fmt.Printf("SFTP transfer of file %s to %s on host %s\n", file, dest, host)
	// return nil

	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	conn, err := ssh.Dial("tcp", host+":"+port, config)
	if err != nil {
		return fmt.Errorf("failed to dial: %v", err)
	}

	defer conn.Close()

	// Start an SFTP session
	client, err := sftp.NewClient(conn)
	if err != nil {
		return fmt.Errorf("failed to create client: %v", err)
	}

	defer client.Close()

	// Open the local file
	srcFile, err := client.Open(file)
	if err != nil {
		return fmt.Errorf("failed to open source file: %v", err)
	}

	defer srcFile.Close()

	// Open the destination file on the remote server
	dstFile, err := client.Create(dest)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %v", err)
	}

	defer dstFile.Close()

	// Copy the file from the local system to the remote server
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy file: %v", err)
	}

	fmt.Printf("File %s transferred to %s on %s\n", file, dest, host)
	return nil
}
