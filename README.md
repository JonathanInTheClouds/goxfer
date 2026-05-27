
# GoXfer: A Secure, Flexible File Transfer Tool

**GoXfer** is a command-line tool written in Go for secure file transfers. The primary workflow is now direct peer-to-peer transfer with a simple `send` / `receive` flow, end-to-end encrypted sessions, checksum verification, and optional resume support. Traditional SFTP, SCP, and FTPS transfers are still available when you need them.

## Features

- **Peer-to-Peer Transfers**: Share files directly with `goxfer send` and `goxfer receive`.
- **End-to-End Encryption**: Establish secure sessions between sender and receiver.
- **Checksum Verification**: Verify file integrity after transfer using SHA256 checksums.
- **Resumable Transfers**: Restart interrupted single-file transfers with `--resume`.
- **Alternate Transfer Modes**: Use SFTP, SCP, or FTPS when direct transfer is not the right fit.

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Peer-to-Peer Usage](#peer-to-peer-usage)
- [Alternate Transfer Modes](#alternate-transfer-modes)
- [Alternate Transfer Options](#alternate-transfer-options)
- [Checksum Verification](#checksum-verification)
- [Contributing](#contributing)
- [License](#license)

## Installation

### Install From Releases

Download the archive for your platform from the [Releases](https://github.com/JonathanInTheClouds/goxfer/releases) page.

#### macOS and Linux

Extract the archive and move `goxfer` somewhere on your `PATH`, for example:

```bash
tar -xzf goxfer_0.1.0_linux_amd64.tar.gz
chmod +x goxfer
sudo mv goxfer /usr/local/bin/goxfer
```

Verify the install:

```bash
goxfer --help
```

#### Windows

Download the matching `.zip` release asset, extract it, and place `goxfer.exe` somewhere convenient such as:

- a tools directory you already use
- a folder added to your `PATH`

Then verify it from PowerShell:

```powershell
goxfer.exe --help
```

### Build From Source

#### Prerequisites

- **Go**: Make sure you have Go installed. You can install it from [here](https://golang.org/doc/install).

#### Clone the Repository

```bash
git clone https://github.com/JonathanInTheClouds/goxfer.git
cd goxfer
```

#### Build the Project

```bash
go build -o goxfer cmd/main.go
```

This will create a binary named `goxfer` in your project directory.

## Quick Start

The new default flow is a sender sharing a generated address, then a receiver connecting to it.

On the sending machine:

```bash
./goxfer send ./file-transfer-container
```

GoXfer will print a `goxfer receive ...` command for the other side to run.

On the receiving machine:

```bash
./goxfer receive bore.pub:49152 ./downloads
```

If you are transferring a single file and want interrupted transfers to resume cleanly, enable `--resume` on both sides:

```bash
./goxfer send --resume ./large-file.iso
./goxfer receive --resume bore.pub:49152 ./downloads
```

## Peer-to-Peer Usage

### Default Relay

By default, `goxfer send` opens a temporary public address through `bore.pub` and waits for the receiver to connect.

```bash
./goxfer send ./path/to/file-or-directory
```

The receiver uses the printed address:

```bash
./goxfer receive <address> ./destination-directory
```

### Self-Hosted Relay

If you want to avoid the default relay, you can run your own:

```bash
./goxfer relay --addr=:7835
```

Then send through that relay:

```bash
./goxfer send --relay=your-relay-host:7835 ./path/to/file
```

The receiver uses the printed command, which includes the relay address and session code.

### Notes

- `--resume` works for single-file transfers when both sender and receiver enable it.
- Directory transfers are streamed as a `.tar.gz` archive and extracted on receipt.
- Both sides print a session fingerprint so the transfer can be verified out of band if needed.

## Alternate Transfer Modes

GoXfer still supports protocol-based remote transfers for environments where peer-to-peer transfer is not practical.

### SFTP Example

```bash
./goxfer --protocol=sftp --host=localhost --port=2222 --username=transferuser --key=/home/jonathan/.ssh/id_rsa --srcPath=./file-transfer-container --destDir=/home/transferuser/file-transfer-container --parallel=5 --retries=3
```

This transfers the files from `./file-transfer-container` to `/home/transferuser/file-transfer-container` on the remote server using SFTP.

## Alternate Transfer Options

| Option            | Description                                                                                     | Default    |
|-------------------|-------------------------------------------------------------------------------------------------|------------|
| `--protocol`      | The protocol to use for file transfer (currently supports `sftp`).                               | `sftp`     |
| `--host`          | The hostname or IP of the remote server.                                                         |            |
| `--port`          | The port on which to connect to the remote server.                                               | `22`       |
| `--username`      | The SSH username to authenticate with.                                                           |            |
| `--password`      | The SSH password to authenticate with (optional if using SSH key authentication).                |            |
| `--key`           | The path to your SSH private key file (optional, required if using key-based authentication).    |            |
| `--srcPath`       | The local file or directory to transfer.                                                         |            |
| `--destDir`       | The destination directory on the remote server.                                                  |            |
| `--parallel`      | The number of parallel transfers to run simultaneously.                                          | `5`        |
| `--retries`       | The maximum number of retries in case of checksum mismatch.                                      | `3`        |

## Checksum Verification

GoXfer automatically verifies file integrity with SHA256 checksums.

- In peer-to-peer mode, sender and receiver compare checksums before the transfer is considered successful.
- In alternate transfer modes, GoXfer compares local and remote files and retries on mismatch up to the configured retry count.

## Dockerized SFTP Server for Testing

You can use a Dockerized SFTP server to test the file transfer functionality. Here’s how to set it up:

### Build and Run the Docker SFTP Server

In the `file-transfer-container` folder, run the following commands to build and launch the SFTP server:

```bash
docker build -t file-transfer-test .
docker run -p 2222:22 -d file-transfer-test
```

### Example Transfer Using the Dockerized SFTP Server

```bash
./goxfer --protocol=sftp --host=localhost --port=2222 --username=transferuser --key=/path/to/private_key --srcPath=/path/to/local/files --destDir=/home/transferuser/file-transfer-container --parallel=5 --retries=3
```

This will transfer files to the Docker container acting as the SFTP server.

## Contributing

Contributions are welcome! Feel free to submit pull requests or open issues if you encounter any problems or have suggestions for improvements.

### To contribute:

1. Fork the repository.
2. Create a new branch for your feature/bug fix.
3. Submit a pull request.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
