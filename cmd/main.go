package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/JonathanInTheClouds/goxfer/internal/transfer"
	"github.com/JonathanInTheClouds/goxfer/internal/tunnel"
)

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "send":
			runSend(os.Args[2:])
			return
		case "receive":
			runReceive(os.Args[2:])
			return
		case "relay":
			runRelay(os.Args[2:])
			return
		}
	}

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

func runSend(args []string) {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	relayAddr := fs.String("relay", "", "Self-hosted relay address (default: use bore.pub)")
	resume := fs.Bool("resume", false, "Enable resumable transfer (both sides must use this flag)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: goxfer send [--relay=host:port] [--resume] <srcPath>")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}
	if err := transfer.P2PSend(fs.Arg(0), *relayAddr, *resume); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runReceive(args []string) {
	fs := flag.NewFlagSet("receive", flag.ExitOnError)
	code := fs.String("code", "", "Session code for self-hosted relay (not needed for bore.pub)")
	resume := fs.Bool("resume", false, "Enable resumable transfer (both sides must use this flag)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: goxfer receive [--code=<code>] [--resume] <address> <destDir>")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	if fs.NArg() != 2 {
		fs.Usage()
		os.Exit(1)
	}
	if err := transfer.P2PReceive(fs.Arg(0), fs.Arg(1), *code, *resume); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runRelay(args []string) {
	fs := flag.NewFlagSet("relay", flag.ExitOnError)
	addr := fs.String("addr", fmt.Sprintf(":%d", tunnel.DefaultRelayPort), "Address to listen on")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: goxfer relay [--addr=:7835]")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	if err := tunnel.RunRelay(*addr); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
