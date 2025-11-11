#!/bin/bash
# =============================================================================
# QNAS BUILD SCRIPT - Filesystem Scanner for QNAP NAS
# =============================================================================
# Author: Claude Code Assistant
# Description: Build optimized binaries for QNAP NAS deployment
# Usage: ./build-qnap.sh [options]
# =============================================================================

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_NAME="scandir"
VERSION="2.0-optimized"
BUILD_DATE=$(date -u +"%Y-%m-%d %H:%M:%S UTC")
GO_VERSION="1.23.3"
DOCKER_IMAGE="qnap-${PROJECT_NAME}-builder"
OUTPUT_DIR="${SCRIPT_DIR}/qnap-build"
PACKAGE_NAME="qnap-${PROJECT_NAME}-${VERSION}"

# Default options
BUILD_TYPE="release"
CLEAN_BUILD=false
VERBOSE=false
SKIP_TESTS=false
CREATE_PACKAGE=true
PUSH_TO_REGISTRY=false
REGISTRY_URL=""

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
QNAS Build Script - Filesystem Scanner for QNAP NAS

USAGE:
    ./build-qnap.sh [OPTIONS]

OPTIONS:
    -h, --help              Show this help message
    -t, --type TYPE         Build type: 'debug' or 'release' (default: release)
    -c, --clean             Clean build (remove old artifacts)
    -v, --verbose           Verbose output
    --skip-tests            Skip running tests
    --no-package            Skip creating deployment package
    --push                  Push to container registry
    --registry URL          Container registry URL
    --go-version VERSION    Go version to use (default: 1.23.3)
    --version VERSION       Build version (default: 2.0-optimized)

EXAMPLES:
    ./build-qnap.sh                                    # Standard release build
    ./build-qnap.sh --type debug --verbose            # Debug build with verbose
    ./build-qnap.sh --clean --skip-tests              # Clean build without tests
    ./build-qnap.sh --push --registry my-registry.com  # Build and push to registry

EOF
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            -t|--type)
                BUILD_TYPE="$2"
                shift 2
                ;;
            -c|--clean)
                CLEAN_BUILD=true
                shift
                ;;
            -v|--verbose)
                VERBOSE=true
                shift
                ;;
            --skip-tests)
                SKIP_TESTS=true
                shift
                ;;
            --no-package)
                CREATE_PACKAGE=false
                shift
                ;;
            --push)
                PUSH_TO_REGISTRY=true
                shift
                ;;
            --registry)
                REGISTRY_URL="$2"
                shift 2
                ;;
            --go-version)
                GO_VERSION="$2"
                shift 2
                ;;
            --version)
                VERSION="$2"
                shift 2
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
    log_step "Validating build requirements..."

    # Check if Docker is available
    if ! command -v docker &> /dev/null; then
        log_error "Docker is required but not installed"
        exit 1
    fi

    # Check Docker daemon is running
    if ! docker info &> /dev/null; then
        log_error "Docker daemon is not running"
        exit 1
    fi

    # Check if we're in the right directory
    if [[ ! -f "go.mod" ]] || [[ ! -f "Dockerfile.qnap" ]]; then
        log_error "Please run this script from the project root directory"
        exit 1
    fi

    # Validate build type
    if [[ "$BUILD_TYPE" != "debug" && "$BUILD_TYPE" != "release" ]]; then
        log_error "Build type must be 'debug' or 'release'"
        exit 1
    fi

    # Validate Go version format
    if [[ ! "$GO_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        log_error "Invalid Go version format: $GO_VERSION"
        exit 1
    fi

    log_success "Requirements validation passed"
}

# Clean old artifacts
clean_build() {
    if [[ "$CLEAN_BUILD" == true ]]; then
        log_step "Cleaning old build artifacts..."

        # Remove Docker images
        if docker images | grep -q "$DOCKER_IMAGE"; then
            log_info "Removing old Docker images..."
            docker rmi -f $(docker images "$DOCKER_IMAGE" -q) 2>/dev/null || true
        fi

        # Remove build directory
        if [[ -d "$OUTPUT_DIR" ]]; then
            log_info "Removing build directory: $OUTPUT_DIR"
            rm -rf "$OUTPUT_DIR"
        fi

        # Remove package files
        rm -f "${SCRIPT_DIR}/qnap-scanner-optimized.tar.gz"
        rm -f "${SCRIPT_DIR}/${PACKAGE_NAME}.tar.gz"

        log_success "Clean completed"
    fi
}

# Run tests
run_tests() {
    if [[ "$SKIP_TESTS" == false ]]; then
        log_step "Running tests..."

        if command -v go &> /dev/null; then
            # Local Go installation available
            if go test ./...; then
                log_success "All tests passed"
            else
                log_error "Tests failed"
                exit 1
            fi
        else
            log_warning "Go not found locally, skipping tests"
        fi
    else
        log_warning "Skipping tests as requested"
    fi
}

# Build Docker image
build_docker_image() {
    log_step "Building QNAS Docker image..."

    local build_args=(
        "--build-arg" "GO_VERSION=${GO_VERSION}"
        "--build-arg" "TARGETARCH=amd64"
        "--build-arg" "BUILD_DATE=${BUILD_DATE}"
        "-t" "${DOCKER_IMAGE}:latest"
        "-t" "${DOCKER_IMAGE}:${VERSION}"
        "-f" "Dockerfile.qnap"
    )

    if [[ "$VERBOSE" == true ]]; then
        build_args+=("--progress=plain")
    fi

    if [[ "$BUILD_TYPE" == "debug" ]]; then
        build_args+=("--target" "qnap-builder")
    fi

    log_info "Building with arguments: ${build_args[*]}"

    if docker build "${build_args[@]}" .; then
        log_success "Docker image built successfully"
    else
        log_error "Docker image build failed"
        exit 1
    fi
}

# Extract binaries from Docker image
extract_binaries() {
    log_step "Extracting binaries from Docker image..."

    mkdir -p "$OUTPUT_DIR"

    local binaries=("scanner" "deleter" "reporter" "reporter_opt")
    local container_name="qnap-extract-$$"

    # Create temporary container
    if docker create --name "$container_name" "${DOCKER_IMAGE}:latest"; then
        log_info "Created temporary container: $container_name"
    else
        log_error "Failed to create temporary container"
        exit 1
    fi

    # Extract binaries
    for binary in "${binaries[@]}"; do
        log_info "Extracting $binary..."
        if docker cp "$container_name:/$binary" "$OUTPUT_DIR/$binary"; then
            chmod +x "$OUTPUT_DIR/$binary"
            log_success "Extracted $binary"
        else
            log_warning "Failed to extract $binary (may not exist)"
        fi
    done

    # Extract deployment package if requested
    if [[ "$CREATE_PACKAGE" == true ]]; then
        log_info "Extracting deployment package..."
        if docker cp "$container_name:/qnap-scanner-optimized.tar.gz" "${SCRIPT_DIR}/"; then
            log_success "Extracted deployment package"
        else
            log_warning "Failed to extract deployment package"
        fi
    fi

    # Cleanup container
    docker rm "$container_name" >/dev/null 2>&1

    log_success "Binary extraction completed"
}

# Verify binaries
verify_binaries() {
    log_step "Verifying extracted binaries..."

    local binaries=("scanner" "deleter" "reporter" "reporter_opt")
    local all_valid=true

    for binary in "${binaries[@]}"; do
        local binary_path="$OUTPUT_DIR/$binary"

        if [[ -f "$binary_path" ]]; then
            # Check if binary is executable
            if [[ -x "$binary_path" ]]; then
                local size=$(stat -c%s "$binary_path" 2>/dev/null || stat -f%z "$binary_path" 2>/dev/null || echo "unknown")
                log_success "$binary: ✓ Executable ($((${size:-0} / 1024))KB"
            else
                log_error "$binary: ✗ Not executable"
                all_valid=false
            fi
        else
            log_warning "$binary: Not found"
        fi
    done

    if [[ "$all_valid" == true ]]; then
        log_success "All binaries verified successfully"
    else
        log_error "Binary verification failed"
        exit 1
    fi
}

# Create deployment package
create_deployment_package() {
    if [[ "$CREATE_PACKAGE" == false ]]; then
        log_warning "Skipping deployment package creation"
        return
    fi

    log_step "Creating QNAS deployment package..."

    local package_dir="${OUTPUT_DIR}/${PACKAGE_NAME}"
    mkdir -p "$package_dir"

    # Copy binaries
    cp "$OUTPUT_DIR"/* "$package_dir/" 2>/dev/null || true

    # Copy configuration
    cp config.ini "$package_dir/config.ini.example" 2>/dev/null || true

    # Create deployment scripts
    cat > "$package_dir/deploy.sh" << 'EOF'
#!/bin/sh
# QNAS Deployment Script
set -e

echo "🚀 Deploying Filesystem Scanner to QNAP NAS..."

# Detect QNAP paths
QPKG_DIR=""
for path in "/share/CACHEDEV1_DATA/.qpkg" "/share/MD0_DATA/.qpkg" "/share/HDA_DATA/.qpkg"; do
    if [[ -d "$path" ]]; then
        QPKG_DIR="$path/scanner"
        break
    fi
done

if [[ -z "$QPKG_DIR" ]]; then
    echo "⚠️  Could not detect QNAP QPKG directory, using default"
    QPKG_DIR="/tmp/scanner"
    mkdir -p "$QPKG_DIR"
fi

# Create directories
mkdir -p "$QPKG_DIR"/{bin,config,data,logs,output}

# Copy binaries
cp scanner deleter reporter reporter_opt "$QPKG_DIR/bin/"
chmod +x "$QPKG_DIR/bin/"*

# Copy configuration
if [[ ! -f "$QPKG_DIR/config/config.ini" ]]; then
    cp config.ini.example "$QPKG_DIR/config/config.ini"
    echo "📝 Created default configuration"
fi

echo "✅ Deployment complete!"
echo "📍 Installation: $QPKG_DIR"
echo "📖 Configuration: $QPKG_DIR/config/config.ini"
echo "🏃 Run: $QPKG_DIR/bin/scanner"
EOF

    chmod +x "$package_dir/deploy.sh"

    # Create README
    cat > "$package_dir/README.md" << EOF
# Filesystem Scanner for QNAP NAS

## Version: $VERSION
## Build Date: $BUILD_DATE

## Quick Start

1. Copy this package to your QNAP NAS
2. Run: \`./deploy.sh\`
3. Edit: \`config.ini\` to set your scan paths
4. Run: \`./bin/scanner\`

## Files

- \`scanner\` - Main filesystem scanner
- \`deleter\` - Database cleanup tool
- \`reporter\` - Basic report generator
- \`reporter_opt\` - Optimized report generator
- \`config.ini.example\` - Example configuration

## Support

For detailed documentation, please refer to the project repository.
EOF

    # Create tar.gz package
    tar -czf "${OUTPUT_DIR}/${PACKAGE_NAME}.tar.gz" -C "$OUTPUT_DIR" "$PACKAGE_NAME"

    log_success "Created deployment package: ${OUTPUT_DIR}/${PACKAGE_NAME}.tar.gz"
}

# Push to registry (if requested)
push_to_registry() {
    if [[ "$PUSH_TO_REGISTRY" == false ]]; then
        return
    fi

    if [[ -z "$REGISTRY_URL" ]]; then
        log_error "Registry URL required for pushing"
        exit 1
    fi

    log_step "Pushing to container registry..."

    local full_image="${REGISTRY_URL}/${DOCKER_IMAGE}:${VERSION}"

    # Tag and push
    if docker tag "${DOCKER_IMAGE}:${VERSION}" "$full_image"; then
        if docker push "$full_image"; then
            log_success "Pushed to registry: $full_image"
        else
            log_error "Failed to push to registry"
            exit 1
        fi
    else
        log_error "Failed to tag image for registry"
        exit 1
    fi
}

# Show build summary
show_summary() {
    log_step "Build Summary"

    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Project:        $PROJECT_NAME"
    echo "Version:        $VERSION"
    echo "Build Type:     $BUILD_TYPE"
    echo "Go Version:     $GO_VERSION"
    echo "Build Date:     $BUILD_DATE"
    echo "Output Directory: $OUTPUT_DIR"
    echo ""

    if [[ -d "$OUTPUT_DIR" ]]; then
        echo "Generated Files:"
        ls -la "$OUTPUT_DIR" | awk 'NR>1 {printf "  %-20s %s\n", $9, $5}'
    fi

    if [[ "$CREATE_PACKAGE" == true && -f "${OUTPUT_DIR}/${PACKAGE_NAME}.tar.gz" ]]; then
        local package_size=$(stat -c%s "${OUTPUT_DIR}/${PACKAGE_NAME}.tar.gz" 2>/dev/null || stat -f%z "${OUTPUT_DIR}/${PACKAGE_NAME}.tar.gz" 2>/dev/null || echo "unknown")
        echo ""
        echo "Deployment Package: ${PACKAGE_NAME}.tar.gz ($((${package_size:-0} / 1024))KB)"
    fi

    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    log_success "QNAS build completed successfully! 🎉"
}

# Main execution
main() {
    echo -e "${CYAN}"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "🚀 QNAS BUILD SCRIPT - Filesystem Scanner for QNAP NAS"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo -e "${NC}"

    parse_args "$@"
    validate_requirements
    clean_build
    run_tests
    build_docker_image
    extract_binaries
    verify_binaries
    create_deployment_package
    push_to_registry
    show_summary
}

# Handle script interruption
trap 'log_error "Build interrupted"; exit 1' INT TERM

# Run main function with all arguments
main "$@"