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
	port := flag.String("port", "", "Remote server port (default: 22 for sftp/scp, 21 for ftps)")
	username := flag.String("username", "", "SSH Username")
	password := flag.String("password", "", "SSH Password (optional if using key)")
	key := flag.String("key", "", "SSH Private Key file path (optional)")
	srcPath := flag.String("srcPath", "", "Source file or directory to transfer")
	destDir := flag.String("destDir", "", "Destination directory on the server")
	maxParallel := flag.Int("parallel", 5, "Max number of parallel transfers")
	maxRetries := flag.Int("retries", 3, "Max number of retry attempts in case of checksum mismatch")
	scpMkdir := flag.Bool("scp-mkdir", false, "Create destination directory if it doesn't exist (only for SCP)")
	insecure := flag.Bool("insecure", false, "Skip host key verification (not recommended for production)")
	knownHosts := flag.String("known-hosts", "~/.ssh/known_hosts", "Path to known_hosts file for host key verification")

	flag.Parse()

	if *port == "" {
		if *protocol == "ftps" {
			*port = "21"
		} else {
			*port = "22"
		}
	}

	if *host == "" || *srcPath == "" || *destDir == "" || *username == "" {
		fmt.Println("Error: host, username, source path, and destination directory must be specified.")
		flag.Usage()
		os.Exit(1)
	}

	fmt.Printf("Starting transfer using %s protocol with up to %d parallel transfers and %d retries...\n", *protocol, *maxParallel, *maxRetries)

	switch *protocol {
	case "sftp":
		err := transfer.SFTPTransfer(*username, *password, *host, *port, *key, *srcPath, *destDir, *knownHosts, *maxParallel, *maxRetries, *insecure)
		if err != nil {
			fmt.Printf("Error transferring: %v\n", err)
			os.Exit(1)
		}
	case "scp":
		err := transfer.SCPTransfer(*username, *password, *host, *port, *key, *srcPath, *destDir, *scpMkdir, *insecure)
		if err != nil {
			fmt.Printf("Error transferring: %v\n", err)
			os.Exit(1)
		}
	case "ftps":
		err := transfer.FTPSTransfer(*username, *password, *host, *port, *srcPath, *destDir, *maxRetries, *insecure)
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
