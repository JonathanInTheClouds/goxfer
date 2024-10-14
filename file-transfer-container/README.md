
# File Transfer Test Container

This directory contains the Docker setup and instructions for creating a server to test file transfers using protocols such as SFTP and SCP in a Docker container. You can use this container to test file transfers with the **GoXfer** tool.

## Building the Docker Container

To build the file transfer test container, run the following command from the root of the project directory:

```bash
docker build -t file-transfer-test ./file-transfer-container
```

This command uses the Dockerfile located in the `file-transfer-container` folder to build the image.

## Running the Docker Container

To run the container and expose the SSH port (22) on port 2222 of the host machine:

```bash
docker run -d -p 2222:22 --name file-transfer-test file-transfer-test
```

This command starts the container in the background (`-d`) and maps port `2222` on the host machine to port `22` in the container.

## Example: Transferring the `README.md` File Using SFTP

Use the **GoXfer** tool to transfer the `README.md` file from your local machine to the Docker container using SFTP. The file will be placed in the `file-transfer-container` directory on the container.

Here’s how you can transfer the `README.md` file:

```bash
go run cmd/main.go --protocol=sftp --host=localhost --port=2222 --username=transferuser --password=transferpassword --file=./file-transfer-container/README.md --dest=/home/transferuser/file-transfer-container/README.md
```

- `--file=./file-transfer-container/README.md`: This specifies the path to the local `README.md` file.
- `--dest=/home/transferuser/file-transfer-container/README.md`: This is the destination path inside the container.

## Example: Transferring the `README.md` File Using SCP

You can also use the **GoXfer** tool to transfer the `README.md` file using SCP:

```bash
go run cmd/main.go --protocol=scp --host=localhost --port=2222 --username=transferuser --key=/path/to/private_key --srcPath=./file-transfer-container/README.md --destDir=/home/transferuser/file-transfer-container/README.md
```

## Verifying the File Transfer

After running the transfer command, you can SSH into the container to verify that the file has been transferred:

```bash
ssh transferuser@localhost -p 2222
ls /home/transferuser/file-transfer-container
```

You should see the `README.md` file inside the `file-transfer-container` folder.

## Stopping and Removing the Container

To stop and remove the container when you're done testing:

```bash
docker stop file-transfer-test
docker rm file-transfer-test
```
