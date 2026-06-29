# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies.
RUN go mod download

# Copy the source code
COPY . .

# Build the application
# CGO_ENABLED=0 ensures a static binary
# -ldflags="-w -s" reduces binary size
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o drop .

# Final stage
FROM alpine:latest

# Add ca-certificates for HTTPS requests (e.g. to R2)
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/drop .

# Expose port (Cloud Run uses 8080 by default)
EXPOSE 8080

# Command to run the executable
CMD ["./drop"]
