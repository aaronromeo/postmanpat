#!/bin/bash

# Source me!

eval "$(ssh-agent -s)"
ssh-add /home/codespace/.ssh/do_tf

echo $SSH_AUTH_SOCK
