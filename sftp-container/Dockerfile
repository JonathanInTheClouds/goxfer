# Base image
FROM ubuntu:20.04

# Install OpenSSH Server
RUN apt-get update && apt-get install -y openssh-server

# Create SSH directory
RUN mkdir /var/run/sshd

# Set up a user for SFTP with password authentication
RUN useradd -m -d /home/sftpuser -s /bin/bash sftpuser && \
    echo "sftpuser:sftppassword" | chpasswd

# Allow password authentication
RUN sed -i 's/#PasswordAuthentication yes/PasswordAuthentication yes/' /etc/ssh/sshd_config
RUN sed -i 's/PermitRootLogin prohibit-password/PermitRootLogin yes/' /etc/ssh/sshd_config

# Expose port 22 for SSH
EXPOSE 22

# Start SSH service
CMD ["/usr/sbin/sshd", "-D"]
