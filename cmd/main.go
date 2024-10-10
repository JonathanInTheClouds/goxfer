package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/JonathanInTheClouds/goxfer/internal/transfer"
)

func main() {
	// Define the flags for protocol, host, port, etc.
	protocol := flag.String("protocol", "sftp", "Protocol to use for file transfer (e.g., sftp, scp, ftps)")
	host := flag.String("host", "", "Remote server host")
	port := flag.String("port", "22", "Remote server port")
	username := flag.String("username", "", "SSH Username")
	password := flag.String("password", "", "SSH Password (optional if using key)")
	key := flag.String("key", "", "SSH Private Key file path (optional)")
	srcPath := flag.String("srcPath", "", "Source file or directory to transfer")
	destDir := flag.String("destDir", "", "Destination directory on the server")

	// Parse the flags
	flag.Parse()

	// Validate the flags
	if *host == "" || *port == "" || *srcPath == "" || *destDir == "" || *username == "" {
		fmt.Println("Error: host, username, source path, and destination directory must be specified.")
		flag.Usage()
		os.Exit(1)
	}

	fmt.Printf("Starting transfer using %s protocol...\n", *protocol)

	// Transfer the file or directory
	switch *protocol {
	case "sftp":
		err := transfer.SFTPTransfer(*username, *password, *host, *port, *key, *srcPath, *destDir)
		if err != nil {
			fmt.Printf("Error transferring: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("Error: unsupported protocol %s\n", *protocol)
		os.Exit(1)
	}

	fmt.Println("Transfer complete.")
}
