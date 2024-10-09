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
	password := flag.String("password", "", "SSH Password")
	file := flag.String("file", "", "File to transfer")
	dest := flag.String("dest", "", "Destination path on the server")

	// Parse the flags
	flag.Parse()

	// Validate the flags
	if *host == "" || *port == "" || *file == "" || *dest == "" || *username == "" || *password == "" {
		fmt.Println("Error: host, username, password, file, and destination must be specified.")
		flag.Usage()
		os.Exit(1)
	}
	fmt.Printf("Starting transfer using %s protocol...\n", *protocol)
	fmt.Printf("Transferring file: %s to %s on %s\n", *file, *dest, *host)

	// Call the transfer function
	switch *protocol {
	case "sftp":
		err := transfer.SFTPTransfer(*username, *password, *host, *port, *file, *dest)
		if err != nil {
			fmt.Printf("Error transferring file: %v\n", err)
			os.Exit(1)
		}
	case "scp":
		err := transfer.SCPTransfer(*file, *host, *dest)
		if err != nil {
			fmt.Printf("Error transferring file: %v\n", err)
			os.Exit(1)
		}
	case "ftps":
		err := transfer.FTPSTransfer(*file, *host, *dest)
		if err != nil {
			fmt.Printf("Error transferring file: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("Error: unsupported protocol %s\n", *protocol)
		os.Exit(1)
	}

	fmt.Println("File transfer complete.")
	os.Exit(0)
}
