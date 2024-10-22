#/bin/bash
echo "Host *\n\tStrictHostKeyChecking no\n\tUserKnownHostsFile=/dev/null" >> /etc/ssh/ssh_config

apt update
apt install -y git openssh-client
npm install -g pnpm
pnpm install
