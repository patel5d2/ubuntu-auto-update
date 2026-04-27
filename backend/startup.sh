#!/bin/sh

# Wait for the database to be ready
until pg_isready -h postgres -p 5432 -U user; do
  echo "Waiting for database..."
  sleep 2
done

# Run migrations
/usr/local/bin/migrate -path /migrations -database postgres://user:password@postgres:5432/uau_db?sslmode=disable up

# Start the application
/ua-backend
