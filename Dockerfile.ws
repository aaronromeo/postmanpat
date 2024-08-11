# Start with the official Golang image
FROM golang:1.23rc1-bullseye AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source code into the container
COPY . .

# Build the Go app
RUN make build

# Stage 2: Create the final image with Node.js
FROM node:20-bullseye

# Copy the built Go app from the builder stage
COPY --from=builder /app /app

# Set the Current Working Directory inside the container
WORKDIR /app

# Build the Node Web Service app
RUN make ws-build

# Install cron
RUN apt-get update && apt-get install -y cron

# Copy over the crontab
COPY crontab /etc/cron.d/postmanpat-crontab

# Updating the crontab file perms
RUN chmod 0644 /etc/cron.d/postmanpat-crontab && crontab /etc/cron.d/postmanpat-crontab

# # Expose port 8080 to the outside world
EXPOSE 3000

# Run the crontab
ENTRYPOINT ["/app/build/postmanpat", "ws"]
