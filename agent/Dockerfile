# Multi-stage build for Ubuntu Auto-Update Agent
# Stage 1: Build the Rust binary
FROM rust:1.75-slim as builder

# Install system dependencies for building
RUN apt-get update && apt-get install -y \
    pkg-config \
    libssl-dev \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Create app user
RUN useradd -m -u 1001 appuser

# Set the working directory
WORKDIR /usr/src/app

# Copy Cargo files first for better layer caching
COPY Cargo.toml Cargo.lock ./

# Create dummy main.rs to cache dependencies
RUN mkdir src && echo "fn main() {}" > src/main.rs

# Build dependencies only
RUN cargo build --release
RUN rm src/main.rs

# Copy the actual source code
COPY src ./src

# Build the application with security optimizations
RUN cargo build --release --locked \
    && strip target/release/ua-agent

# Verify the binary
RUN ldd target/release/ua-agent 2>/dev/null || true

# Stage 2: Create the runtime image
FROM ubuntu:22.04

LABEL maintainer="Ubuntu Auto-Update Team"
LABEL description="Memory-safe Ubuntu update agent with enterprise security"
LABEL version="0.2.0"

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    ca-certificates \
    apt-utils \
    lsb-release \
    curl \
    gnupg \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user (will be overridden for actual updates)
RUN useradd -m -u 1001 -s /bin/bash appuser

# Create necessary directories
RUN mkdir -p /etc/ubuntu-auto-update \
    /var/log/ubuntu-auto-update \
    /var/lib/node_exporter/textfile_collector \
    && chown -R appuser:appuser /etc/ubuntu-auto-update \
    && chown -R appuser:appuser /var/log/ubuntu-auto-update \
    && chmod 755 /var/lib/node_exporter/textfile_collector

# Copy the binary from builder stage
COPY --from=builder /usr/src/app/target/release/ua-agent /usr/local/bin/ua-agent
RUN chmod +x /usr/local/bin/ua-agent

# Copy configuration template
COPY docker/agent.toml /etc/ubuntu-auto-update/agent.toml.example

# Copy entrypoint script
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# Security: Use minimal attack surface
# Note: For actual updates, container will need to be run as root
USER appuser

# Set environment variables
ENV RUST_LOG=info
ENV UA_LOGGING__LEVEL=info
ENV UA_LOGGING__FORMAT=json

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ua-agent test || exit 1

# Default command
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["status"]

# Labels for OCI compliance
LABEL org.opencontainers.image.title="Ubuntu Auto-Update Agent"
LABEL org.opencontainers.image.description="Enterprise-grade Ubuntu update agent"
LABEL org.opencontainers.image.version="0.2.0"
LABEL org.opencontainers.image.source="https://github.com/patel5d2/ubuntu-auto-update"
LABEL org.opencontainers.image.licenses="MIT"
LABEL org.opencontainers.image.vendor="Ubuntu Auto-Update Team"