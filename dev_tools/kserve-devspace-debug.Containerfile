FROM golang:1.24-bookworm

# Install bash just to be sure
RUN apt-get update && apt-get install -y bash sudo

# Create directories with full access
RUN mkdir -p /app /.cache /.vscode-server /.config /home/vscode && \
    chmod -R 777 /app /.cache /usr/local/bin /.vscode-server /.config /home/vscode

# Create user with bash and setup sudo
RUN useradd -m -s /bin/bash vscode && \
    echo "vscode ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers

ENV HOME=/home/vscode

# Switch to the user
USER vscode
WORKDIR /app
