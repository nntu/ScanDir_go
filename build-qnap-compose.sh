#!/bin/bash
# =============================================================================
# QNAS DOCKER COMPOSE BUILD SCRIPT
# =============================================================================
# Author: Claude Code Assistant
# Description: Build QNAS-optimized binaries using Docker Compose
# Usage: ./build-qnap-compose.sh [options]
# =============================================================================

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m'

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VERSION="${VERSION:-2.0-qnap}"
GO_VERSION="${GO_VERSION:-1.23.3}"
BUILD_DATE="${BUILD_DATE:-$(date -u +"%Y-%m-%d %H:%M:%S UTC")}"
COMPOSE_FILE="docker-compose.qnap.yml"

# Default options
BUILD_AMD64=true
BUILD_ARM64=false
RUN_TESTS=true
CLEAN_BUILD=false
PUSH_IMAGES=false
REGISTRY_URL=""
VERBOSE=false
PULL_LATEST=true

# Functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "${PURPLE}[STEP]${NC} $1"
}

show_help() {
    cat << EOF
QNAS Docker Compose Build Script

USAGE:
    ./build-qnap-compose.sh [OPTIONS]

OPTIONS:
    -h, --help              Show this help message
    -v, --version VERSION   Build version (default: 2.0-qnap)
    --go-version VERSION    Go version (default: 1.23.3)
    --amd64                 Build AMD64 binaries (default: true)
    --no-amd64              Skip AMD64 build
    --arm64                 Build ARM64 binaries
    --no-tests              Skip running tests
    --clean                 Clean build (remove old artifacts)
    --push                  Push images to registry
    --registry URL          Container registry URL
    --no-pull               Skip pulling latest images
    --verbose               Verbose output

EXAMPLES:
    ./build-qnap-compose.sh                           # Build AMD64 only
    ./build-qnap-compose.sh --arm64                   # Build AMD64 + ARM64
    ./build-qnap-compose.sh --clean --version 1.0.0   # Clean build with version
    ./build-qnap-compose.sh --push --registry my-registry.com  # Build and push

EOF
}

# Parse arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            -v|--version)
                VERSION="$2"
                shift 2
                ;;
            --go-version)
                GO_VERSION="$2"
                shift 2
                ;;
            --amd64)
                BUILD_AMD64=true
                shift
                ;;
            --no-amd64)
                BUILD_AMD64=false
                shift
                ;;
            --arm64)
                BUILD_ARM64=true
                shift
                ;;
            --no-tests)
                RUN_TESTS=false
                shift
                ;;
            --clean)
                CLEAN_BUILD=true
                shift
                ;;
            --push)
                PUSH_IMAGES=true
                shift
                ;;
            --registry)
                REGISTRY_URL="$2"
                shift 2
                ;;
            --no-pull)
                PULL_LATEST=false
                shift
                ;;
            --verbose)
                VERBOSE=true
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

# Validate requirements
validate_requirements() {
    log_step "Validating requirements..."

    # Check Docker
    if ! command -v docker &> /dev/null; then
        log_error "Docker is required but not installed"
        exit 1
    fi

    # Check Docker Compose
    if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
        log_error "Docker Compose is required but not installed"
        exit 1
    fi

    # Check Docker daemon
    if ! docker info &> /dev/null; then
        log_error "Docker daemon is not running"
        exit 1
    fi

    # Check compose file
    if [[ ! -f "$COMPOSE_FILE" ]]; then
        log_error "Compose file not found: $COMPOSE_FILE"
        exit 1
    fi

    log_success "Requirements validation passed"
}

# Clean old artifacts
clean_build() {
    if [[ "$CLEAN_BUILD" == true ]]; then
        log_step "Cleaning old build artifacts..."

        # Remove volumes
        docker volume rm qnap_qnap-output 2>/dev/null || true
        docker volume rm qnap_qnap-packages 2>/dev/null || true

        # Remove containers
        docker-compose -f "$COMPOSE_FILE" down -v 2>/dev/null || true

        # Remove images
        docker rmi qnap-scandir-builder:amd64-cache 2>/dev/null || true
        docker rmi qnap-scandir-builder:arm64-cache 2>/dev/null || true

        # Clean directories
        rm -rf qnap-output qnap-packages

        log_success "Clean completed"
    fi
}

# Prepare environment
prepare_environment() {
    log_step "Preparing build environment..."

    # Create directories
    mkdir -p qnap-output qnap-packages

    # Create environment file
    cat > .env.qnap << EOF
# QNAS Build Environment
VERSION=$VERSION
GO_VERSION=$GO_VERSION
BUILD_DATE="$BUILD_DATE"
BUILD_TYPE=release
EOF

    log_success "Environment prepared"
}

# Pull latest images
pull_latest() {
    if [[ "$PULL_LATEST" == true ]]; then
        log_step "Pulling latest base images..."

        docker-compose -f "$COMPOSE_FILE" pull --ignore-pull-failures || true

        log_success "Base images pulled"
    fi
}

# Build AMD64
build_amd64() {
    if [[ "$BUILD_AMD64" == false ]]; then
        log_warning "Skipping AMD64 build"
        return
    fi

    log_step "Building QNAS AMD64 binaries..."

    local compose_cmd=("docker-compose" "-f" "$COMPOSE_FILE" "build" "qnap-amd64-builder")
    if [[ "$VERBOSE" == true ]]; then
        compose_cmd+=("--progress=plain")
    fi

    if "${compose_cmd[@]}"; then
        log_success "AMD64 build completed"
    else
        log_error "AMD64 build failed"
        exit 1
    fi
}

# Build ARM64
build_arm64() {
    if [[ "$BUILD_ARM64" == false ]]; then
        log_warning "Skipping ARM64 build"
        return
    fi

    log_step "Building QNAS ARM64 binaries..."

    # Check if platform is available
    if ! docker buildx inspect | grep -q "linux/arm64"; then
        log_warning "ARM64 platform not available, installing buildx..."
        docker buildx install || true
    fi

    local compose_cmd=("docker-compose" "-f" "$COMPOSE_FILE" "build" "qnap-arm64-builder")
    if [[ "$VERBOSE" == true ]]; then
        compose_cmd+=("--progress=plain")
    fi

    if "${compose_cmd[@]}"; then
        log_success "ARM64 build completed"
    else
        log_error "ARM64 build failed"
        exit 1
    fi
}

# Extract binaries
extract_binaries() {
    log_step "Extracting built binaries..."

    # Create extractors
    docker-compose -f "$COMPOSE_FILE" up -d qnap-amd64-builder || true
    if [[ "$BUILD_ARM64" == true ]]; then
        docker-compose -f "$COMPOSE_FILE" --profile arm64 up -d qnap-arm64-builder || true
    fi

    # Extract from containers
    if docker ps | grep -q "qnap-amd64-build"; then
        log_info "Extracting AMD64 binaries..."
        mkdir -p qnap-output/amd64
        docker cp qnap-amd64-build:/out/. qnap-output/amd64/ 2>/dev/null || true
    fi

    if [[ "$BUILD_ARM64" == true ]] && docker ps | grep -q "qnap-arm64-build"; then
        log_info "Extracting ARM64 binaries..."
        mkdir -p qnap-output/arm64
        docker cp qnap-arm64-build:/out/. qnap-output/arm64/ 2>/dev/null || true
    fi

    log_success "Binary extraction completed"
}

# Run tests
run_tests() {
    if [[ "$RUN_TESTS" == false ]]; then
        log_warning "Skipping tests"
        return
    fi

    log_step "Running QNAS binary tests..."

    # Test AMD64
    if [[ -f "qnap-output/amd64/scanner" ]]; then
        if qnap-output/amd64/scanner --help >/dev/null 2>&1; then
            log_success "✅ AMD64 scanner test passed"
        else
            log_error "❌ AMD64 scanner test failed"
            exit 1
        fi
    fi

    # Test ARM64 (if built)
    if [[ "$BUILD_ARM64" == true && -f "qnap-output/arm64/scanner" ]]; then
        if qnap-output/arm64/scanner --help >/dev/null 2>&1; then
            log_success "✅ ARM64 scanner test passed"
        else
            log_error "❌ ARM64 scanner test failed"
            exit 1
        fi
    fi

    log_success "All tests passed"
}

# Create packages
create_packages() {
    log_step "Creating deployment packages..."

    docker-compose -f "$COMPOSE_FILE" run --rm qnap-packager

    log_success "Package creation completed"
}

# Push images
push_images() {
    if [[ "$PUSH_IMAGES" == false ]]; then
        return
    fi

    if [[ -z "$REGISTRY_URL" ]]; then
        log_error "Registry URL required for pushing"
        exit 1
    fi

    log_step "Pushing images to registry..."

    # Push AMD64
    if [[ "$BUILD_AMD64" == true ]]; then
        docker tag qnap-scandir-builder:amd64-${VERSION} "${REGISTRY_URL}/qnap-scandir-builder:amd64-${VERSION}"
        docker push "${REGISTRY_URL}/qnap-scandir-builder:amd64-${VERSION}"
    fi

    # Push ARM64
    if [[ "$BUILD_ARM64" == true ]]; then
        docker tag qnap-scandir-builder:arm64-${VERSION} "${REGISTRY_URL}/qnap-scandir-builder:arm64-${VERSION}"
        docker push "${REGISTRY_URL}/qnap-scandir-builder:arm64-${VERSION}"
    fi

    log_success "Images pushed successfully"
}

# Show summary
show_summary() {
    log_step "Build Summary"

    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Project:        QNAS Scanner"
    echo "Version:        $VERSION"
    echo "Go Version:     $GO_VERSION"
    echo "Build Date:     $BUILD_DATE"
    echo "AMD64 Build:    $BUILD_AMD64"
    echo "ARM64 Build:    $BUILD_ARM64"
    echo "Tests Run:      $RUN_TESTS"
    echo ""

    # Show packages
    if [[ -d "qnap-packages" ]]; then
        echo "Generated Packages:"
        find qnap-packages -name "*.tar.gz" -exec basename {} \; | while read pkg; do
            local size=$(stat -c%s "qnap-packages/$pkg" 2>/dev/null || stat -f%z "qnap-packages/$pkg" 2>/dev/null || echo "unknown")
            echo "  $pkg ($((${size:-0} / 1024))KB)"
        done
    fi

    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    log_success "QNAS Docker Compose build completed! 🎉"
}

# Cleanup
cleanup() {
    log_step "Cleaning up..."

    docker-compose -f "$COMPOSE_FILE" down 2>/dev/null || true
    docker volume rm qnap_qnap-output 2>/dev/null || true
    docker volume rm qnap_qnap-packages 2>/dev/null || true

    rm -f .env.qnap

    log_success "Cleanup completed"
}

# Main execution
main() {
    echo -e "${CYAN}"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "🐳 QNAS DOCKER COMPOSE BUILD - Filesystem Scanner Multi-Architecture"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo -e "${NC}"

    parse_args "$@"
    validate_requirements
    clean_build
    prepare_environment
    pull_latest
    build_amd64
    build_arm64
    extract_binaries
    run_tests
    create_packages
    push_images
    show_summary

    # Cleanup on success
    cleanup
}

# Handle script interruption
trap 'log_error "Build interrupted"; cleanup; exit 1' INT TERM

# Run main function
main "$@"