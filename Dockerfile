# Stage 1: Builder
FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Install necessary build tools
RUN apk add --no-cache git

# Copy go mod files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Retrieve version and commit information
ARG VERSION
ARG COMMIT

# Build the application
RUN CGO_ENABLED=0 \
    go build -tags pebbledb \
    -ldflags "-s -w \
    -X github.com/binaryholdings/cosmos-pruner/cmd.Version=${VERSION} \
    -X github.com/binaryholdings/cosmos-pruner/cmd.Commit=${COMMIT}" \
    -o /cosmprund main.go

# Stage 2: Minimal runtime
FROM alpine:3.21.3

# Install CA certificates
RUN apk add --no-cache ca-certificates

# Copy the compiled binary from the builder stage
COPY --from=builder /cosmprund /usr/local/bin/cosmprund

# Set the entrypoint
ENTRYPOINT ["/usr/local/bin/cosmprund"]
