# Multi-stage build for eventcron
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
RUN mkdir -p /var/spool/eventcron \
    && mkdir -p /etc/eventcron.d \
    && mkdir -p /var/run

# Create eventcron user and group
RUN addgroup -g 1000 eventcron \
    && adduser -D -u 1000 -G eventcron eventcron

# Copy binaries from builder
COPY --from=builder /src/eventcrond /usr/sbin/eventcrond
COPY --from=builder /src/eventcrontab /usr/bin/eventcrontab

# Set proper permissions
RUN chmod 755 /usr/sbin/eventcrond \
    && chmod 4755 /usr/bin/eventcrontab \
    && chown root:root /usr/sbin/eventcrond /usr/bin/eventcrontab

# Set proper directory permissions
RUN chown root:root /var/spool/eventcron \
    && chmod 755 /var/spool/eventcron \
    && chown root:root /etc/eventcron.d \
    && chmod 755 /etc/eventcron.d

# Create default config file
RUN echo "# eventcron configuration file" > /etc/eventcron.conf \
    && echo "# This file is currently unused but reserved for future configuration options" >> /etc/eventcron.conf

# Create volume mount points
VOLUME ["/var/spool/eventcron", "/etc/eventcron.d", "/watch"]

# Expose no ports (local file system monitoring)

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD pgrep eventcrond > /dev/null || exit 1

# Default command - run daemon in foreground
CMD ["/usr/sbin/eventcrond", "-n"]

# Metadata
LABEL maintainer="eventcron project" \
      description="Inotify cron daemon written in Go" \
      version="1.0.0" \
      org.opencontainers.image.title="eventcron" \
      org.opencontainers.image.description="Modern Go implementation of inotify cron system" \
      org.opencontainers.image.source="https://github.com/dpvpro/eventcron" \
      org.opencontainers.image.licenses="GPL-3.0"