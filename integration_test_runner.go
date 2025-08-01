//go:build integration
// +build integration

package serverless

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"net/http"
)

// TestServerlessIntegrationSuite runs the complete integration test suite
func TestServerlessIntegrationSuite(t *testing.T) {
	// Check if Docker is available
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer func() { _ = cli.Close() }()

	ctx := context.Background()
	if _, pingErr := cli.Ping(ctx); pingErr != nil {
		t.Skipf("Cannot connect to Docker daemon: %v", pingErr)
	}

	// Check if test images exist
	images := []string{
		"caddy-serverless-go-echoserver-test:latest",
		"caddy-serverless-py-echoserver-test:latest",
	}

	for _, img := range images {
		if !imageExists(ctx, cli, img) {
			t.Skipf("Test image %s not found. Please build test images first.", img)
		}
	}

	// Start Caddy server
	caddyCmd, err := startCaddyServer()
	if err != nil {
		t.Fatalf("Failed to start Caddy server: %v", err)
	}
	defer func() {
		if caddyCmd != nil && caddyCmd.Process != nil {
			_ = caddyCmd.Process.Kill()
		}
	}()

	// Wait for server to be ready
	if waitErr := waitForServer("http://localhost:8080/health", 30*time.Second); waitErr != nil {
		t.Fatalf("Caddy server did not start: %v", waitErr)
	}

	// Run test suite
	suite, err := NewTestSuite(t, "http://localhost:8080")
	if err != nil {
		t.Fatalf("Failed to create test suite: %v", err)
	}

	suite.RunAllTests()
}

func imageExists(ctx context.Context, cli *client.Client, imageName string) bool {
	images, err := cli.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return false
	}

	for _, img := range images {
		for _, tag := range img.RepoTags {
			if tag == imageName {
				return true
			}
		}
	}
	return false
}

func startCaddyServer() (*exec.Cmd, error) {
	// Look for Caddy binary
	caddyPath := "./caddy"
	if _, err := os.Stat(caddyPath); os.IsNotExist(err) {
		caddyPath = "../../../caddy" // Try from module directory
		if _, err := os.Stat(caddyPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("caddy binary not found")
		}
	}

	// Look for test config
	configPath := "test-serverless.Caddyfile"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = "../../../test-serverless.Caddyfile"
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("test configuration not found")
		}
	}

	cmd := exec.Command(caddyPath, "run", "--config", configPath, "--adapter", "caddyfile")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return cmd, nil
}

func waitForServer(url string, timeout time.Duration) error {
	client := &http.Client{Timeout: 1 * time.Second}
	start := time.Now()

	for time.Since(start) < timeout {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == 200 {
			_ = resp.Body.Close()
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("server did not respond within %v", timeout)
}