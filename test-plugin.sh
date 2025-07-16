#!/bin/bash

# Test script for validating the Caddy serverless plugin as an external module

set -e

echo "=== Caddy Serverless Plugin Test Suite ==="
echo

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check prerequisites
check_prerequisites() {
    echo "Checking prerequisites..."
    
    # Check for Go
    if ! command -v go &> /dev/null; then
        echo -e "${RED}✗ Go is not installed${NC}"
        exit 1
    else
        echo -e "${GREEN}✓ Go is installed: $(go version)${NC}"
    fi
    
    # Check for Docker
    if ! command -v docker &> /dev/null; then
        echo -e "${RED}✗ Docker is not installed${NC}"
        exit 1
    else
        echo -e "${GREEN}✓ Docker is installed: $(docker --version)${NC}"
    fi
    
    # Check for xcaddy
    if ! command -v xcaddy &> /dev/null; then
        echo -e "${YELLOW}! xcaddy is not installed, installing...${NC}"
        go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest
        export PATH=$PATH:$(go env GOPATH)/bin
    else
        echo -e "${GREEN}✓ xcaddy is installed${NC}"
    fi
    
    echo
}

# Test 1: Build Caddy with the plugin
test_build() {
    echo "Test 1: Building Caddy with the serverless plugin..."
    
    # Clean previous build
    rm -f caddy
    
    # Build with xcaddy
    if xcaddy build --with github.com/jose/caddy-serverless=.; then
        echo -e "${GREEN}✓ Build successful${NC}"
        return 0
    else
        echo -e "${RED}✗ Build failed${NC}"
        return 1
    fi
}

# Test 2: Verify module loads
test_module_loads() {
    echo
    echo "Test 2: Verifying serverless module loads..."
    
    if ./caddy list-modules | grep -q "http.handlers.serverless"; then
        echo -e "${GREEN}✓ Serverless module loaded successfully${NC}"
        return 0
    else
        echo -e "${RED}✗ Serverless module not found${NC}"
        echo "Available modules:"
        ./caddy list-modules | grep http.handlers || true
        return 1
    fi
}

# Test 3: Validate Caddyfile parsing
test_caddyfile_parsing() {
    echo
    echo "Test 3: Testing Caddyfile parsing..."
    
    # Create a test Caddyfile
    cat > test.Caddyfile <<EOF
{
    order serverless before file_server
}

localhost:8080 {
    serverless {
        function {
            methods GET POST
            path /test
            image alpine:latest
            command echo "test"
            port 8080
        }
    }
}
EOF
    
    if ./caddy validate --config test.Caddyfile --adapter caddyfile; then
        echo -e "${GREEN}✓ Caddyfile validation successful${NC}"
        rm -f test.Caddyfile
        return 0
    else
        echo -e "${RED}✗ Caddyfile validation failed${NC}"
        rm -f test.Caddyfile
        return 1
    fi
}

# Test 4: Run unit tests
test_unit_tests() {
    echo
    echo "Test 4: Running unit tests..."
    
    if go test -v -race ./...; then
        echo -e "${GREEN}✓ Unit tests passed${NC}"
        return 0
    else
        echo -e "${RED}✗ Unit tests failed${NC}"
        return 1
    fi
}

# Test 5: Build Docker test images
test_docker_images() {
    echo
    echo "Test 5: Building Docker test images..."
    
    # Build Go echo server
    echo "Building Go echo server test image..."
    if docker build -t caddy-serverless-go-echoserver-test:latest ./testdata/echoserver/; then
        echo -e "${GREEN}✓ Go echo server image built${NC}"
    else
        echo -e "${RED}✗ Failed to build Go echo server image${NC}"
        return 1
    fi
    
    # Build Python echo server
    echo "Building Python echo server test image..."
    if docker build -t caddy-serverless-py-echoserver-test:latest ./testdata/pyechoserver/; then
        echo -e "${GREEN}✓ Python echo server image built${NC}"
    else
        echo -e "${RED}✗ Failed to build Python echo server image${NC}"
        return 1
    fi
    
    return 0
}

# Test 6: Run integration tests
test_integration() {
    echo
    echo "Test 6: Running integration tests (if Docker images are available)..."
    
    # Check if test images exist
    if docker images | grep -q "caddy-serverless-go-echoserver-test"; then
        if go test -v -tags=integration -timeout=10m ./...; then
            echo -e "${GREEN}✓ Integration tests passed${NC}"
            return 0
        else
            echo -e "${RED}✗ Integration tests failed${NC}"
            return 1
        fi
    else
        echo -e "${YELLOW}! Skipping integration tests (test images not found)${NC}"
        return 0
    fi
}

# Test 7: Test with example configuration
test_example_config() {
    echo
    echo "Test 7: Testing with example Caddyfile..."
    
    # Start Caddy in the background
    ./caddy start --config example.Caddyfile --adapter caddyfile &> caddy.log &
    CADDY_PID=$!
    
    # Wait for Caddy to start
    sleep 3
    
    # Check if Caddy is running
    if ps -p $CADDY_PID > /dev/null; then
        echo -e "${GREEN}✓ Caddy started successfully${NC}"
        
        # Test health endpoint if configured
        if curl -s -f http://localhost:8080/health > /dev/null 2>&1; then
            echo -e "${GREEN}✓ Health endpoint responding${NC}"
        fi
        
        # Stop Caddy
        kill $CADDY_PID 2>/dev/null || true
        wait $CADDY_PID 2>/dev/null || true
        rm -f caddy.log
        return 0
    else
        echo -e "${RED}✗ Caddy failed to start${NC}"
        echo "Caddy logs:"
        cat caddy.log || true
        rm -f caddy.log
        return 1
    fi
}

# Main test execution
main() {
    local total_tests=0
    local passed_tests=0
    
    check_prerequisites
    
    # Run tests
    tests=(
        "test_build"
        "test_module_loads"
        "test_caddyfile_parsing"
        "test_unit_tests"
        "test_docker_images"
        "test_integration"
        "test_example_config"
    )
    
    for test in "${tests[@]}"; do
        ((total_tests++))
        if $test; then
            ((passed_tests++))
        fi
    done
    
    # Summary
    echo
    echo "=== Test Summary ==="
    echo "Total tests: $total_tests"
    echo "Passed: $passed_tests"
    echo "Failed: $((total_tests - passed_tests))"
    
    if [ $passed_tests -eq $total_tests ]; then
        echo -e "${GREEN}✓ All tests passed!${NC}"
        exit 0
    else
        echo -e "${RED}✗ Some tests failed${NC}"
        exit 1
    fi
}

# Run main
main