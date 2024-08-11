#!/bin/bash

# Enable strict mode
set -euxo pipefail

# Update package list
sudo apt-get update -y

# Install Docker if not already installed
if ! command -v docker &> /dev/null; then
  sudo apt-get install -y docker.io
  sudo systemctl start docker
  sudo systemctl enable docker
fi

# Install docker-compose if not already installed
if ! command -v docker-compose &> /dev/null; then
  sudo curl -L "https://github.com/docker/compose/releases/download/2.29.1/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
  sudo chmod +x /usr/local/bin/docker-compose
fi

# Run watchtower container if not already running
if ! sudo docker ps --format '{{.Names}}' | grep -w watchtower &> /dev/null; then
  sudo docker run -d --name watchtower -v /var/run/docker.sock:/var/run/docker.sock containrrr/watchtower --cleanup
fi

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
