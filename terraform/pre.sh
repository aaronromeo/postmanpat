#!/bin/bash

# Source me!

# sudo
mkdir -p /home/codespace/.ssh/

# sudo
chown -R codespace:codespace /home/codespace/.ssh/

# export DO_TF_PRIVATE_KEY=$(sed ':a;N;$!ba;s/\n/\\n/g' do_tf)
# export DO_TF_PUBLIC_KEY=$(sed ':a;N;$!ba;s/\n/\\n/g' do_tf.pub)
echo $DO_TF_PRIVATE_KEY | sed 's/\\n/\n/g' > /home/codespace/.ssh/do_tf
echo $DO_TF_PUBLIC_KEY | sed 's/\\n/\n/g' > /home/codespace/.ssh/do_tf.pub
chmod 600 /home/codespace/.ssh/do_tf
chmod 600 /home/codespace/.ssh/do_tf.pub

eval "$(ssh-agent -s)"
ssh-add /home/codespace/.ssh/do_tf

echo $SSH_AUTH_SOCK
