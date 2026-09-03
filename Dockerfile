# ── Build stage ────────────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache gcc musl-dev git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -tags fts5 -ldflags="-s -w" -o /mira ./cmd/mira

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

RUN adduser -D -u 1000 mira
USER mira
WORKDIR /home/mira

COPY --from=builder /mira /usr/local/bin/mira

# Default data directory
RUN mkdir -p /home/mira/.mira

EXPOSE 3001 8080 9090

ENTRYPOINT ["mira"]
CMD ["--config", "/home/mira/config.yaml"]
