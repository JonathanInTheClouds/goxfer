
# SFTP Test Container

This directory contains the Docker setup and instructions for creating an SFTP server in a Docker container. You can use this container to test file transfers with the **GoXfer** tool.

## Building the Docker Container

To build the SFTP container, run the following command from the root of the project directory:

```bash
docker build -t sftp-test ./sftp-container
```

This command uses the Dockerfile located in the `sftp-container` folder to build the image.

## Running the Docker Container

To run the container and expose the SSH port (22) on port 2222 of the host machine:

```bash
docker run -d -p 2222:22 --name sftp-container sftp-test
```

This command starts the container in the background (`-d`) and maps port `2222` on the host machine to port `22` in the container.

## Example: Transferring the `README.md` File

Use the **GoXfer** tool to transfer the `README.md` file from your local machine to the Docker container. The file will be placed in the `sftp-container` directory on the container.

Here’s how you can transfer the `README.md` file:

```bash
go run cmd/main.go --protocol=sftp --host=localhost --port=2222 --username=sftpuser --password=sftppassword --file=./sftp-container/README.md --dest=/home/sftpuser/sftp-container/README.md
```

- `--file=./sftp-container/README.md`: This specifies the path to the local `README.md` file.
- `--dest=/home/sftpuser/sftp-container/README.md`: This is the destination path inside the container.

## Verifying the File Transfer

After running the transfer command, you can SSH into the container to verify that the file has been transferred:

```bash
ssh sftpuser@localhost -p 2222
ls /home/sftpuser/sftp-container
```

You should see the `README.md` file inside the `sftp-container` folder.

## Stopping and Removing the Container

To stop and remove the container when you're done testing:

```bash
docker stop sftp-container
docker rm sftp-container
```
