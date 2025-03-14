#!/bin/bash

set -e # Exit immediately if a command exits with a non-zero status

echo "====== Setting up Docker and NVIDIA components on Ubuntu 22.04 ======"

# Function to display messages with timestamp
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

# Check if script is run as root
if [ "$(id -u)" -ne 0 ]; then
    log "This script must be run as root or with sudo privileges"
    exit 1
fi

# Capture username of the sudo user for later use
if [ -n "$SUDO_USER" ]; then
    CURRENT_USER=$SUDO_USER
else
    CURRENT_USER=$(whoami)
    if [ "$CURRENT_USER" = "root" ]; then
        log "Please provide the username to add to the docker group:"
        read -r CURRENT_USER
    fi
fi

log "Installing for user: $CURRENT_USER"

# ====== 1. Install Docker Engine ======
log "Installing Docker Engine..."

# Remove old versions if they exist
apt-get remove -y docker docker-engine docker.io containerd runc || true

# Update apt package index
apt-get update

# Install packages to allow apt to use a repository over HTTPS
apt-get install -y \
    apt-transport-https \
    ca-certificates \
    curl \
    gnupg \
    lsb-release \
    software-properties-common

# Add Docker's official GPG key
log "Adding Docker GPG key..."
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg

# Set up the Docker repository
echo \
    "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list >/dev/null

# Install Docker Engine
log "Installing Docker packages..."
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin

# Start and enable Docker service
systemctl enable --now docker

# ====== 2 & 3. Install NVIDIA GPU Driver and CUDA 12.4 ======
log "Installing NVIDIA GPU drivers and CUDA 12.4..."

# Install essential build tools
apt-get install -y build-essential dkms

# Add NVIDIA package repository
curl -fsSL https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2204/x86_64/cuda-keyring_1.1-1_all.deb -o cuda-keyring.deb
dpkg -i cuda-keyring.deb
rm cuda-keyring.deb

# Update the apt repository cache
apt-get update

# Install CUDA 12.4 which includes the drivers
log "Installing CUDA 12.4..."
apt-get install -y cuda-12-4 cuda-drivers

# ====== 4. Install NVIDIA Container Runtime Toolkit ======
log "Installing NVIDIA Container Runtime Toolkit..."

# Install the NVIDIA container toolkit
apt-get install -y nvidia-container-toolkit

# Configure Docker to use NVIDIA runtime
log "Configuring Docker to use NVIDIA runtime..."
nvidia-ctk runtime configure --runtime=docker

# Restart Docker to apply changes
systemctl restart docker

# ====== 5. Add the current user to the docker group ======
log "Adding user '$CURRENT_USER' to the docker group..."
usermod -aG docker "$CURRENT_USER"

# ====== 6. Pull required Docker images ======
log "Pulling required Docker images..."
docker pull redis/redis-stack
docker pull sivanantha/lmcahce-vllm:latest
docker pull sivanantha/huggingfaceserver:latest
log "Docker images successfully pulled"

# ====== Finalize ======
log "Setup complete!"
log "IMPORTANT: You need to log out and log back in for the docker group changes to take effect."
log "To verify Docker installation, run: docker run hello-world"
log "To verify NVIDIA setup, run: docker run --rm --gpus all nvidia/cuda:12.4.0-base-ubuntu22.04 nvidia-smi"

# Set environment variables for the current session
echo '
# CUDA Environment Variables
export PATH=/usr/local/cuda-12.4/bin${PATH:+:${PATH}}
export LD_LIBRARY_PATH=/usr/local/cuda-12.4/lib64${LD_LIBRARY_PATH:+:${LD_LIBRARY_PATH}}
' >>/etc/profile.d/cuda.sh

log "CUDA environment variables have been set in /etc/profile.d/cuda.sh"
log "To apply them in your current session, run: source /etc/profile.d/cuda.sh"
