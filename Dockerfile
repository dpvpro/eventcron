# Multi-stage build for eventcrone
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /src

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binaries
RUN make build

# Final stage - minimal runtime image
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    shadow \
    && rm -rf /var/cache/apk/*

# Create necessary directories
RUN mkdir -p /var/spool/eventcrone \
    && mkdir -p /etc/eventcrone.d \
    && mkdir -p /var/run

# Create eventcrone user and group
RUN addgroup -g 1000 eventcrone \
    && adduser -D -u 1000 -G eventcrone eventcrone

# Copy binaries from builder
COPY --from=builder /src/eventcroned /usr/sbin/eventcroned
COPY --from=builder /src/eventcronetab /usr/bin/eventcronetab

# Set proper permissions
RUN chmod 755 /usr/sbin/eventcroned \
    && chmod 4755 /usr/bin/eventcronetab \
    && chown root:root /usr/sbin/eventcroned /usr/bin/eventcronetab

# Set proper directory permissions
RUN chown root:root /var/spool/eventcrone \
    && chmod 755 /var/spool/eventcrone \
    && chown root:root /etc/eventcrone.d \
    && chmod 755 /etc/eventcrone.d

# Create default config file
RUN echo "# eventcrone configuration file" > /etc/eventcrone.conf \
    && echo "# This file is currently unused but reserved for future configuration options" >> /etc/eventcrone.conf

# Create volume mount points
VOLUME ["/var/spool/eventcrone", "/etc/eventcrone.d", "/watch"]

# Expose no ports (local file system monitoring)

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD pgrep eventcroned > /dev/null || exit 1

# Default command - run daemon in foreground
CMD ["/usr/sbin/eventcroned", "-n"]

# Metadata
LABEL maintainer="eventcrone project" \
      description="Inotify cron daemon written in Go" \
      version="1.0.0" \
      org.opencontainers.image.title="eventcrone" \
      org.opencontainers.image.description="Modern Go implementation of inotify cron system" \
      org.opencontainers.image.source="https://github.com/dpvpro/eventcrone" \
      org.opencontainers.image.licenses="GPL-3.0"