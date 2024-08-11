#!/bin/bash

source /etc/profile.d/postmanpat.sh

echo "
IMAP_URL="${IMAP_URL}"
IMAP_USER="${IMAP_USER}"
IMAP_PASS="${IMAP_PASS}"

DIGITALOCEAN_BUCKET_ACCESS_KEY="${DIGITALOCEAN_BUCKET_ACCESS_KEY}"
DIGITALOCEAN_BUCKET_SECRET_KEY="${DIGITALOCEAN_BUCKET_SECRET_KEY}"
" > /tmp/postmanpat.env

# Enable strict mode
set -euxo pipefail

sudo docker login -u ${DIGITALOCEAN_USER} -p ${DIGITALOCEAN_CONTAINER_REGISTRY_TOKEN} registry.digitalocean.com

sudo docker pull registry.digitalocean.com/aaronromeo/postmanpat/cron:latest

sudo docker pull registry.digitalocean.com/aaronromeo/postmanpat/ws:latest

sudo docker system prune -f

# docker compose ps
# https://docs.docker.com/reference/cli/docker/compose/ps/

sudo docker-compose up -d \
    --file /tmp/docker-compose.yml \
    --env-file  /tmp/postmanpat.env \
    --watch
