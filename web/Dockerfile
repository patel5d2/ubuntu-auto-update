# Stage 1: Build the React application
FROM node:18-alpine AS build

WORKDIR /app

COPY package.json ./

RUN npm install

COPY . .

RUN npm run build

# Stage 2: Serve the application using Nginx
FROM nginx:1.23-alpine

# Copy the built assets from the build stage
COPY --from=build /app/dist /usr/share/nginx/html

# Copy nginx config
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
