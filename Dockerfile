# Start with the official Golang image
FROM golang:1.23rc1-bullseye

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

# Expose port 8080 to the outside world
EXPOSE 8080

# Command to run the executable
CMD ["/app/build/postmanpat"]
# CMD [ "/bin/ls -a" ]
