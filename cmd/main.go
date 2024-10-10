package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/JonathanInTheClouds/goxfer/internal/transfer"
)

func main() {
	protocol := flag.String("protocol", "sftp", "Protocol to use for file transfer (e.g., sftp, scp, ftps)")
	host := flag.String("host", "", "Remote server host")
	port := flag.String("port", "22", "Remote server port")
	username := flag.String("username", "", "SSH Username")
	password := flag.String("password", "", "SSH Password (optional if using key)")
	key := flag.String("key", "", "SSH Private Key file path (optional)")
	srcPath := flag.String("srcPath", "", "Source file or directory to transfer")
	destDir := flag.String("destDir", "", "Destination directory on the server")
	maxParallel := flag.Int("parallel", 5, "Max number of parallel transfers")

	flag.Parse()

	if *host == "" || *srcPath == "" || *destDir == "" || *username == "" {
		fmt.Println("Error: host, username, source path, and destination directory must be specified.")
		flag.Usage()
		os.Exit(1)
	}

	fmt.Printf("Starting transfer using %s protocol with up to %d parallel transfers...\n", *protocol, *maxParallel)

	switch *protocol {
	case "sftp":
		err := transfer.SFTPTransfer(*username, *password, *host, *port, *key, *srcPath, *destDir, *maxParallel)
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
