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

sudo docker pull registry.digitalocean.com/aaronromeo/postmanpat:latest

if sudo docker ps -q -f name=postmanpat | grep -q .; then
    sudo docker stop postmanpat
fi

# Remove the existing container if it exists
if sudo docker ps -a -q -f name=postmanpat | grep -q .; then
    sudo docker rm postmanpat
fi

sudo docker run -d \
    --env-file /tmp/postmanpat.env \
    --name postmanpat \
    --log-driver json-file \
    --log-opt max-size=10m \
    --log-opt max-file=3 \
    -p 80:80 registry.digitalocean.com/aaronromeo/postmanpat:latest
