FROM node:22

WORKDIR /app

COPY . ./
RUN npm -g i pnpm
RUN pnpm i
RUN npm run build

RUN echo "Host *\n\tStrictHostKeyChecking no\n\tUserKnownHostsFile=/dev/null" >> /etc/ssh/ssh_config

CMD ["npm", "start"]
