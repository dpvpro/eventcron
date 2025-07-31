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
RUN mkdir -p /var/spool/incron \
    && mkdir -p /etc/incron.d \
    && mkdir -p /var/run

# Create incron user and group
RUN addgroup -g 1000 incron \
    && adduser -D -u 1000 -G incron incron

# Copy binaries from builder
COPY --from=builder /src/incrond /usr/sbin/incrond
COPY --from=builder /src/incrontab /usr/bin/incrontab

# Set proper permissions
RUN chmod 755 /usr/sbin/incrond \
    && chmod 4755 /usr/bin/incrontab \
    && chown root:root /usr/sbin/incrond /usr/bin/incrontab

# Set proper directory permissions
RUN chown root:root /var/spool/incron \
    && chmod 755 /var/spool/incron \
    && chown root:root /etc/incron.d \
    && chmod 755 /etc/incron.d

# Create default config file
RUN echo "# eventcrone configuration file" > /etc/incron.conf \
    && echo "# This file is currently unused but reserved for future configuration options" >> /etc/incron.conf

# Create volume mount points
VOLUME ["/var/spool/incron", "/etc/incron.d", "/watch"]

# Expose no ports (local file system monitoring)

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD pgrep incrond > /dev/null || exit 1

# Default command - run daemon in foreground
CMD ["/usr/sbin/incrond", "-n"]

# Metadata
LABEL maintainer="eventcrone project" \
      description="Inotify cron daemon written in Go" \
      version="1.0.0" \
      org.opencontainers.image.title="eventcrone" \
      org.opencontainers.image.description="Modern Go implementation of inotify cron system" \
      org.opencontainers.image.source="https://github.com/dpvpro/incron-next" \
      org.opencontainers.image.licenses="GPL-3.0"