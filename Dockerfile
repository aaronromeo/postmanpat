FROM golang:1.25-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /out/postmanpat ./cmd/postmanpat

FROM debian:bookworm-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates cron \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /out/postmanpat /usr/local/bin/postmanpat
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

CMD ["/entrypoint.sh"]
