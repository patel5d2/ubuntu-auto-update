# syntax=docker/dockerfile:1
#
# Stage 1: build the React bundle.
# Vite 7 requires Node >= 20.
FROM node:20-alpine AS build

WORKDIR /app

COPY package.json package-lock.json* ./
RUN npm install --no-audit --no-fund

COPY . .

# VITE_API_URL is empty because the bundle runs at the same origin as Nginx,
# which proxies /api/ → backend. Same-origin = no CORS preflight needed.
ENV VITE_API_URL=""
RUN npm run build


# Stage 2: serve the static bundle and reverse-proxy /api/ to backend.
FROM nginx:1.27-alpine

COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
