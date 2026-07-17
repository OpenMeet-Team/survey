# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies (including Node.js for frontend)
RUN apk add --no-cache git ca-certificates nodejs npm

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy package files for npm caching
COPY web/package.json web/package-lock.json ./web/

# Install npm dependencies
RUN cd web && npm ci

# Copy source code
COPY . .

# Build frontend assets
RUN cd web && npm run build

# Install templ and generate template files.
# Pin the CLI to the go.mod runtime version (github.com/a-h/templ) so it stays
# buildable on this Go toolchain and `templ generate` matches the runtime.
# `@latest` drifts ahead (v0.3.1020 requires Go >= 1.25, breaking this builder).
# Bump in lockstep with go.mod.
RUN go install github.com/a-h/templ/cmd/templ@v0.3.960
RUN templ generate

# Build both binaries
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /consumer ./cmd/consumer

# Install golang-migrate for database migrations
RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Final stage
FROM alpine:3.20

# Install ca-certificates for HTTPS calls
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -g '' appuser

# Copy binaries from builder
COPY --from=builder /api /usr/local/bin/api
COPY --from=builder /consumer /usr/local/bin/consumer
COPY --from=builder /go/bin/migrate /usr/local/bin/migrate

# Copy migrations for database migrations
COPY --from=builder /app/internal/db/migrations /migrations

# Copy frontend assets
COPY --from=builder /app/web/dist /app/web/dist

# Copy static assets (OG images, etc.)
COPY --from=builder /app/static /app/static

# Set working directory (so relative paths work)
WORKDIR /app

# Use non-root user
USER appuser

# Expose port
EXPOSE 8080

# Default to api, override in k8s for consumer
ENTRYPOINT ["/usr/local/bin/api"]
