#!/bin/bash

# Source me!

echo "Now in: $(pwd)"

# Replace the image tag in the docker-compose file
if [ ! -r "docker-compose.yml" ]; then
    echo "docker-compose.yml is missing"
    return 1
fi

if [ ! -r "docker-compose.yml" ]; then
    echo "docker-compose.yml is missing"
    return 1
fi

# Get the current branch name
BRANCH_NAME=$(git rev-parse --abbrev-ref HEAD)

# Define the SHA or latest tag
if [ "$BRANCH_NAME" == "main" ]; then
    TAG="latest"
else
    # Get the SHA of the latest commit
    TAG=$BRANCH_NAME
fi

sed "s/:latest/:$TAG/g" docker-compose.yml > terraform/workingfiles/docker-compose.yml
sed "s/:latest/:$TAG/g" terraform/provision/update-script.sh > terraform/workingfiles/update-script.sh

mkdir -p /home/codespace/.ssh/
chown -R codespace:codespace /home/codespace/.ssh/

echo $DO_TF_PRIVATE_KEY | sed 's/\\n/\n/g' > /home/codespace/.ssh/do_tf
echo $DO_TF_PUBLIC_KEY | sed 's/\\n/\n/g' > /home/codespace/.ssh/do_tf.pub
chmod 600 /home/codespace/.ssh/do_tf
chmod 600 /home/codespace/.ssh/do_tf.pub

eval "$(ssh-agent -s)"
ssh-add /home/codespace/.ssh/do_tf

echo $SSH_AUTH_SOCK
