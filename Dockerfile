# Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o beer-alerter .

# Runtime stage
# debian:bookworm-slim over Alpine: more reliable Chromium rendering for complex SPAs
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    chromium \
    fonts-liberation \
    libnss3 \
    libatk-bridge2.0-0 \
    libgbm1 \
    libxkbcommon0 \
    libxdamage1 \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -m -u 1001 alerter
USER alerter
WORKDIR /app
COPY --from=builder /build/beer-alerter .

# Mount your config.yaml at /app/config.yaml, or set CONFIG_PATH env var.
# IMPORTANT: run with --shm-size=256m — Chrome needs shared memory for rendering.
# Example:
#   docker run -d --shm-size=256m -v $(pwd)/config.yaml:/app/config.yaml:ro beer-alerter

ENTRYPOINT ["./beer-alerter"]
