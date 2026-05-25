# FTPS Test Container

Docker container running vsftpd for local FTPS testing with GoXfer. Uses explicit FTPS (AUTH TLS on port 21) with a self-signed certificate.

Credentials match the SFTP test container: `transferuser` / `transferpassword`

## Build and Run

```bash
docker build -t ftps-transfer-test ./ftps-test-container
docker run -d -p 21:21 -p 30000-30009:30000-30009 --name ftps-transfer-test ftps-transfer-test
```

The passive port range (30000–30009) must be mapped so data connections can reach the container.

## Transfer Files

Because the certificate is self-signed, pass `--insecure` to skip TLS verification:

```bash
./goxfer \
  --protocol=ftps \
  --host=localhost \
  --username=transferuser \
  --password=transferpassword \
  --srcPath=./file-transfer-container \
  --destDir=/home/transferuser/uploads \
  --retries=3 \
  --insecure
```

## Verify the Transfer

```bash
docker exec ftps-transfer-test find /home/transferuser -type f
```

## Stop and Remove

```bash
docker stop ftps-transfer-test
docker rm ftps-transfer-test
```
