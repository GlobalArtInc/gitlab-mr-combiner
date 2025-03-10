#!/bin/bash
cat <<EOF >> /etc/ssh/ssh_config
Host *
    StrictHostKeyChecking no
    UserKnownHostsFile=/dev/null
EOF


apt update
apt install -y git openssh-client

if [ -f "go.mod" ]; then
  go mod download
fi
