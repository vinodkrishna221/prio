# =====================================================================
# Stage 1: Build the Go binary
# =====================================================================
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Install git and ca-certificates (needed for fetching Go modules and
# making HTTPS requests at runtime).
RUN apk add --no-cache git ca-certificates

# Copy go module files first for layer caching
COPY go.mod go.sum ./

RUN go mod download

# Copy the full gateway source (including pre-generated proto stubs)
COPY . .

# Build a statically-linked binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /server ./cmd/server

# =====================================================================
# Stage 2: Minimal runtime image
# =====================================================================
FROM alpine:3.21

# Install ca-certificates so the binary can make HTTPS calls to Google
# APIs, Convex, Redis (TLS), etc.
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /server /app/server

# Cloud Run injects PORT env var; default to 8080
ENV PORT=8080
EXPOSE 8080

# Run as non-root for security
RUN adduser -D -u 1001 appuser
USER appuser

ENTRYPOINT ["/app/server"]
