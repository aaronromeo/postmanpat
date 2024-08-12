#!/bin/bash

# Enable strict mode
set -euxo pipefail

# Update package list
sudo apt-get update -y

# Install Docker if not already installed
if ! command -v docker &> /dev/null; then
  # Add Docker's official GPG key:
  sudo apt-get update
  sudo apt-get install -y ca-certificates curl
  sudo install -m 0755 -d /etc/apt/keyrings
  sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
  sudo chmod a+r /etc/apt/keyrings/docker.asc

  # Add the repository to Apt sources:
  echo \
    "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu \
    $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
    sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
  sudo apt-get update

  VERSION_STRING=5:27.1.1-1~ubuntu.22.04~jammy
  sudo apt-get install -y docker-ce=$VERSION_STRING docker-ce-cli=$VERSION_STRING containerd.io docker-buildx-plugin docker-compose-plugin

  sudo systemctl start docker
  sudo systemctl enable docker  
fi

# # Install docker-compose if not already installed
# if ! command -v docker-compose &> /dev/null; then
#   sudo curl -L "https://github.com/docker/compose/releases/download/v2.29.1/docker-compose-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m)" -o /usr/local/bin/docker-compose
#   sudo chmod +x /usr/local/bin/docker-compose
# fi

# Install webhook if not already installed
if ! command -v webhook &> /dev/null; then
  sudo apt-get install -y webhook
fi

# Copy over the update script
UPDATE_SCRIPT='/usr/local/bin/update-script.sh'
chmod +x "$UPDATE_SCRIPT"

HOOKS_FILE='/etc/webhook/hooks.json'
# Start webhook if not already running
if ! pgrep -f "webhook -hooks" &> /dev/null; then
  nohup webhook -hooks $HOOKS_FILE -port 9000 &
fi
