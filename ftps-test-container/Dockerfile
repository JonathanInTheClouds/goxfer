FROM ubuntu:20.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y vsftpd openssl && rm -rf /var/lib/apt/lists/*

# Generate a self-signed certificate (use --insecure when connecting with goxfer)
RUN openssl req -x509 -nodes -days 3650 -newkey rsa:2048 \
    -keyout /etc/ssl/private/vsftpd.pem \
    -out /etc/ssl/private/vsftpd.pem \
    -subj "/C=US/ST=Test/L=Test/O=GoXfer/CN=localhost" && \
    chmod 600 /etc/ssl/private/vsftpd.pem

# Create the same user/password as the SFTP test container for consistency
RUN useradd -m -d /home/transferuser -s /bin/bash transferuser && \
    echo "transferuser:transferpassword" | chpasswd

RUN mkdir -p /var/run/vsftpd/empty

COPY vsftpd.conf /etc/vsftpd.conf

# Port 21: FTPS control, 30000-30009: passive data ports
EXPOSE 21 30000-30009

CMD ["/usr/sbin/vsftpd", "/etc/vsftpd.conf"]
