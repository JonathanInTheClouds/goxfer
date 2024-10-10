
# GoXfer: A Secure, Flexible File Transfer Tool

**GoXfer** is a command-line tool written in Go for securely transferring files using various protocols such as SFTP, with support for parallel transfers, checksum verification, retries, and SSH key authentication. 

## Features

- **Parallel File Transfers**: Speed up large transfers with concurrent file transfers.
- **Checksum Verification**: Ensure file integrity after transfer using SHA256 checksum comparison.
- **Retry Mechanism**: Automatically retry transfers in case of checksum mismatches.
- **Passphrase-Protected SSH Key Support**: Authenticate with SSH private keys that are protected by passphrases.
- **Customizable Transfer Options**: Configure the number of parallel transfers, retries, and more.

## Table of Contents

- [Installation](#installation)
- [Usage](#usage)
- [Options](#options)
- [Checksum Verification](#checksum-verification)
- [Contributing](#contributing)
- [License](#license)

## Installation

### Prerequisites

- **Go**: Make sure you have Go installed. You can install it from [here](https://golang.org/doc/install).
  
### Clone the Repository

```bash
git clone https://github.com/JonathanInTheClouds/goxfer.git
cd goxfer
```

### Build the Project

```bash
go build -o goxfer cmd/main.go
```

This will create a binary named `goxfer` in your project directory.

## Usage

To transfer files using GoXfer, you can use the following command:

```bash
./goxfer --protocol=sftp --host=localhost --port=2222 --username=sftpuser --key=/path/to/private_key --srcPath=/path/to/local/files --destDir=/remote/destination/path --parallel=5 --retries=3
```

### Example

```bash
./goxfer --protocol=sftp --host=localhost --port=2222 --username=sftpuser --key=/home/jonathan/.ssh/id_rsa --srcPath=./sftp-container --destDir=/home/sftpuser/sftp-container --parallel=5 --retries=3
```

This example transfers the files from the `./sftp-container` folder to the `/home/sftpuser/sftp-container` folder on the remote server using **SFTP**.

## Options

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

GoXfer automatically verifies file integrity by calculating the SHA256 checksum of both the local and remote files. If the checksums don't match, GoXfer will retry the transfer up to the specified number of retries (default: 3).

## Dockerized SFTP Server for Testing

You can use a Dockerized SFTP server to test the file transfer functionality. Here’s how to set it up:

### Build and Run the Docker SFTP Server

In the `sftp-container` folder, run the following commands to build and launch the SFTP server:

```bash
docker build -t sftp-server .
docker run -p 2222:22 -d sftp-server
```

### Example Transfer Using the Dockerized SFTP Server

```bash
./goxfer --protocol=sftp --host=localhost --port=2222 --username=sftpuser --key=/path/to/private_key --srcPath=/path/to/local/files --destDir=/remote/destination/path --parallel=5 --retries=3
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
