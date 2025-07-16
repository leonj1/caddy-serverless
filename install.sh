#!/bin/bash
# install.sh - Install Caddy with serverless plugin using Docker
# This script builds and installs Caddy without requiring Go on the host

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default installation directory
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BUILD_DIR="./build"

# Function to print colored output
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Check if Docker is installed and running
check_docker() {
    if ! command -v docker &> /dev/null; then
        print_error "Docker is not installed. Please install Docker first."
        echo "Visit: https://docs.docker.com/get-docker/"
        exit 1
    fi

    if ! docker info &> /dev/null; then
        print_error "Docker daemon is not running. Please start Docker."
        exit 1
    fi

    print_info "Docker is installed and running ✓"
}

# Check if running with appropriate permissions
check_permissions() {
    if [[ "$1" == "system" ]] && [[ "$EUID" -ne 0 ]]; then
        print_warning "System-wide installation requires sudo privileges."
        echo "Please run: sudo $0 system"
        exit 1
    fi
}

# Build Caddy with serverless plugin
build_caddy() {
    print_info "Building Caddy with serverless plugin..."
    
    # Create build directory
    mkdir -p "$BUILD_DIR"
    
    # Check if BuildKit is available
    if docker buildx version &> /dev/null; then
        export DOCKER_BUILDKIT=1
        print_info "Using Docker BuildKit for optimized build"
    fi
    
    # Build using Docker
    if DOCKER_BUILDKIT=1 docker build \
        --file Dockerfile.install \
        --output type=local,dest="$BUILD_DIR" \
        --progress=plain \
        . ; then
        
        # Make binary executable
        chmod +x "$BUILD_DIR/caddy"
        print_info "Build completed successfully!"
        
        # Verify the build
        if "$BUILD_DIR/caddy" version &> /dev/null; then
            print_info "Caddy binary verified ✓"
            echo
            "$BUILD_DIR/caddy" version
            echo
        else
            print_error "Failed to verify Caddy binary"
            exit 1
        fi
    else
        print_error "Docker build failed"
        exit 1
    fi
}

# Install Caddy to the specified location
install_caddy() {
    local install_type="$1"
    local target_path
    
    if [[ "$install_type" == "system" ]]; then
        target_path="$INSTALL_DIR/caddy"
        print_info "Installing Caddy system-wide to $target_path"
        
        # Backup existing binary if present
        if [[ -f "$target_path" ]]; then
            print_warning "Existing Caddy binary found. Creating backup..."
            mv "$target_path" "${target_path}.backup.$(date +%Y%m%d_%H%M%S)"
        fi
        
        # Install the binary
        cp "$BUILD_DIR/caddy" "$target_path"
        
        print_info "Installation completed!"
        echo
        echo "Caddy has been installed to: $target_path"
        
    else
        # Local installation
        target_path="$BUILD_DIR/caddy"
        print_info "Caddy binary is available at: $target_path"
        echo
        echo "To install system-wide, run:"
        echo "  sudo $0 system"
        echo
        echo "Or manually copy the binary:"
        echo "  sudo cp $target_path $INSTALL_DIR/caddy"
    fi
    
    # Show how to verify installation
    echo
    echo "To verify the installation:"
    echo "  $target_path version"
    echo "  $target_path list-modules | grep serverless"
}

# Clean up build artifacts
cleanup() {
    if [[ -d "$BUILD_DIR" ]] && [[ "$1" == "--clean" ]]; then
        print_info "Cleaning up build directory..."
        rm -rf "$BUILD_DIR"
    fi
}

# Display usage information
usage() {
    echo "Usage: $0 [COMMAND]"
    echo
    echo "Commands:"
    echo "  local    Build Caddy and keep it in ./build/caddy (default)"
    echo "  system   Build and install Caddy system-wide (requires sudo)"
    echo "  clean    Remove build artifacts"
    echo "  help     Show this help message"
    echo
    echo "Environment Variables:"
    echo "  INSTALL_DIR   Custom installation directory (default: /usr/local/bin)"
    echo
    echo "Examples:"
    echo "  $0              # Build locally"
    echo "  sudo $0 system  # Build and install system-wide"
    echo "  $0 clean        # Clean up build files"
}

# Main script logic
main() {
    local command="${1:-local}"
    
    case "$command" in
        local)
            check_docker
            build_caddy
            install_caddy "local"
            ;;
        system)
            check_permissions "system"
            check_docker
            build_caddy
            install_caddy "system"
            cleanup "--clean"
            ;;
        clean)
            cleanup "--clean"
            print_info "Build directory cleaned"
            ;;
        help|--help|-h)
            usage
            ;;
        *)
            print_error "Unknown command: $command"
            echo
            usage
            exit 1
            ;;
    esac
}

# Run the main function
main "$@"