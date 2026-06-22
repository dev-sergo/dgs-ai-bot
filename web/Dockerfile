# --- build frontend ---
# Собираем web/dist внутри образа: Node нужен только на этой стадии,
# на риге его ставить не требуется.
FROM node:20-alpine AS build
WORKDIR /web
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

# --- serve via nginx ---
# Чистая зона ответственности: nginx отдаёт статику и проксирует /api на Go.
FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /web/dist /usr/share/nginx/html
EXPOSE 80
