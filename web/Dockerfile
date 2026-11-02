FROM node:24-alpine AS build
RUN corepack enable
WORKDIR /app
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ .
# types.test.ts 会引用服务端检查目录用于前后端定义一致性校验，构建期需保持在同样的相对路径
COPY server/internal/inspection/catalog.json /server/internal/inspection/catalog.json
RUN pnpm build
FROM nginx:1.27-alpine
COPY deploy/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
