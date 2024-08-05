# Start with the official Golang image
FROM golang:1.23rc1-bullseye

# Set the Current Working Directory inside the container
WORKDIR /app

# Install cron
RUN apt-get update && apt-get install -y cron

# Copy over the crontab
COPY crontab /etc/cron.d/postmanpat-crontab

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source code into the container
COPY . .

# Build the Go app
RUN make build

# # Expose port 8080 to the outside world
# EXPOSE 8080

# Updating the crontab file perms
RUN chmod 0644 /etc/cron.d/postmanpat-crontab && crontab /etc/cron.d/postmanpat-crontab

# Run the crontab
ENTRYPOINT ["cron", "-f"]
