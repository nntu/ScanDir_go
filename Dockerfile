# Use an older Debian distribution for an older GLIBC
FROM debian:stretch

# Install necessary packages: git (if needed for go modules), build-essential (for CGO)
RUN apt-get update && apt-get install -y \
    git \
    build-essential \
    ca-certificates \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Install Go 1.24.0
ENV GOLANG_VERSION 1.24.0
RUN set -eux; \
    ARCH=; \
    case "$(dpkg --print-architecture)" in \
        amd64) ARCH='amd64';; \
        arm64) ARCH='arm64';; \
        *) echo "unsupported architecture"; exit 1 ;; \
    esac; \
    \
    wget -O go.tgz "https://golang.org/dl/go${GOLANG_VERSION}.linux-${ARCH}.tar.gz"; \
    tar -C /usr/local -xzf go.tgz; \
    rm go.tgz; \
    \
    export PATH="/usr/local/go/bin:$PATH"; \
    go version

ENV PATH="/usr/local/go/bin:$PATH"

# Set the working directory inside the container
WORKDIR /app

# Copy the Go project files into the container
COPY . .

# Build the application
# CGO_ENABLED=1 is crucial here for go-sqlite3
# GOOS=linux is implicit since we are in a Linux container
# GOARCH will be the container's architecture
CMD ["go", "build", "-o", "scanner", "scanner.go"]
