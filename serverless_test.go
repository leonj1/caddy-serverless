package serverless_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2/caddytest"
)

const (
	goEchoServerDir          = "./testdata/echoserver"
	goTestDockerImageName    = "caddy-serverless-go-echoserver-test"
	pyEchoServerDir          = "./testdata/pyechoserver"
	pyTestDockerImageName    = "caddy-serverless-py-echoserver-test"
	commonTestDockerImageTag = "latest"
)

// Helper to build a test Docker image
func buildTestImage(t *testing.T, imageName, imageTag, buildContextDir string) string {
	t.Helper()
	imageFullName := fmt.Sprintf("%s:%s", imageName, imageTag)

	// Check if image already exists
	cmdCheck := exec.Command("docker", "image", "inspect", imageFullName)
	if err := cmdCheck.Run(); err == nil {
		t.Logf("Docker image %s already exists, skipping build", imageFullName)
		return imageFullName
	}

	t.Logf("Building Docker image %s from %s", imageFullName, buildContextDir)
	cmd := exec.Command("docker", "build", "-t", imageFullName, buildContextDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Attempt to remove partial image if build failed
		cleanupCmd := exec.Command("docker", "rmi", imageFullName)
		_ = cleanupCmd.Run() // Ignore error, just best effort
		t.Fatalf("Failed to build Docker image %s: %v", imageFullName, err)
	}
	return imageFullName
}

// Helper to remove the test Docker image
func removeTestImage(t *testing.T, imageName string) {
	t.Helper()
	if imageName == "" {
		return
	}
	t.Logf("Removing Docker image %s", imageName)
	cmd := exec.Command("docker", "rmi", "-f", imageName) // -f to force remove if containers are using it
	if err := cmd.Run(); err != nil {
		// Don't fail the test for cleanup issues, but log it
		t.Logf("Failed to remove Docker image %s: %v. Manual cleanup might be required.", imageName, err)
	}
}

// Structure for the echoserver's response
type EchoResponse struct {
	Headers http.Header `json:"headers"`
	Body    string      `json:"body"`
}

func TestServerlessPlugin_PostEcho(t *testing.T) {
	// Build the Docker image for the echoserver
	// Skip if Docker is not available
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker not found in PATH, skipping integration test")
	}

	imageFullName := buildTestImage(t, goTestDockerImageName, commonTestDockerImageTag, goEchoServerDir)
	// Defer image removal, but only if it was built by this test run (or if we decide to always try removing)
	// For simplicity in this example, we'll always try to remove it.
	// A more robust solution might involve checking if the image existed before the test.
	defer removeTestImage(t, imageFullName)

	// Define Caddy JSON configuration
	// Ensure admin API is configured to listen on caddytest.Default.AdminPort (2999)
	// as caddytest will continue to try and communicate with it on that port.
	caddyJSON := fmt.Sprintf(`
	{
		"admin": {
			"listen": "localhost:2999"
		},
		"apps": {
			"http": {
				"servers": {
					"srv0": {
						"listen": [":9080"],
						"routes": [
							{
								"handle": [{
									"handler": "serverless",
									"functions": [{
										"methods": ["POST"],
										"path": "/echo",
										"image": "%s",
										"port": 8080,
										"timeout": "60s"
									}]
								}]
							}
						]
					}
				}
			}
		}
	}
	`, imageFullName)

	// Initialize Caddy server
	tester := caddytest.NewTester(t)
	tester.InitServer(caddyJSON, "json")
    // defer tester.StopServer() // This was incorrect, caddytest.Tester has no StopServer method. Cleanup is handled by t.Cleanup().


	// Prepare POST request
	requestPayload := `{"message": "hello from caddy test"}`
	requestBody := bytes.NewBufferString(requestPayload)

	// Construct URL based on the configured port in the JSON config
	serverURL := "http://localhost:9080"
	req, err := http.NewRequest("POST", serverURL+"/echo", requestBody)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Custom-Header", "CaddyServerlessTest")
	req.Header.Set("User-Agent", "Caddy-Test-Agent") // To check if User-Agent is passed

	// Send request to Caddy
	client := &http.Client{Timeout: 90 * time.Second} // Increased timeout for Docker startup
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status %d, got %d. Response body: %s", http.StatusOK, resp.StatusCode, string(bodyBytes))
	}

	// Read and unmarshal response body
	responseBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	var echoResp EchoResponse
	if err := json.Unmarshal(responseBodyBytes, &echoResp); err != nil {
		t.Fatalf("Failed to unmarshal response JSON: %v. Body: %s", err, string(responseBodyBytes))
	}

	// Verify headers
	if contentType := echoResp.Headers.Get("Content-Type"); !strings.Contains(strings.ToLower(contentType), "application/json") {
		// The echoserver itself sets Content-Type: application/json for its *response*
		// Here we are checking the *request* headers that were echoed back.
		// The original request to Caddy had Content-Type: application/json
		originalRequestContentType := echoResp.Headers.Get("Content-Type") // This is the Content-Type of the request *to the echoserver*
		if !strings.Contains(strings.ToLower(originalRequestContentType), "application/json") {
			t.Errorf("Expected echoed 'Content-Type' header to contain 'application/json', got '%s'", originalRequestContentType)
		}
	}
	if customHeader := echoResp.Headers.Get("X-Custom-Header"); customHeader != "CaddyServerlessTest" {
		t.Errorf("Expected echoed 'X-Custom-Header' to be 'CaddyServerlessTest', got '%s'", customHeader)
	}
    if userAgent := echoResp.Headers.Get("User-Agent"); userAgent != "Caddy-Test-Agent" {
		t.Errorf("Expected echoed 'User-Agent' to be 'Caddy-Test-Agent', got '%s'", userAgent)
	}


	// Verify body
	if echoResp.Body != requestPayload {
		t.Errorf("Expected echoed body to be '%s', got '%s'", requestPayload, echoResp.Body)
	}

	t.Log("Serverless POST echo test completed successfully.")
}

func TestServerlessPlugin_PythonPostEcho(t *testing.T) {
	// Skip if Docker is not available
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker not found in PATH, skipping integration test")
	}

	imageFullName := buildTestImage(t, pyTestDockerImageName, commonTestDockerImageTag, pyEchoServerDir)
	defer removeTestImage(t, imageFullName)

	// Define Caddy JSON configuration
	// Ensure admin API is configured to listen on caddytest.Default.AdminPort (2999)
	// as caddytest will continue to try and communicate with it on that port.
	caddyJSON := fmt.Sprintf(`
	{
		"admin": {
			"listen": "localhost:2999"
		},
		"apps": {
			"http": {
				"servers": {
					"srv0": {
						"listen": [":9080"],
						"routes": [
							{
								"handle": [{
									"handler": "serverless",
									"functions": [{
										"methods": ["POST"],
										"path": "/pyecho",
										"image": "%s",
										"port": 8080,
										"timeout": "90s"
									}]
								}]
							}
						]
					}
				}
			}
		}
	}
	`, imageFullName) // Increased timeout for Python/Flask cold start

	tester := caddytest.NewTester(t)
	tester.InitServer(caddyJSON, "json")
	// defer tester.StopServer() // This was incorrect.

	requestPayload := `{"message": "hello from caddy python test"}`
	requestBody := bytes.NewBufferString(requestPayload)

	// Construct URL based on the configured port in the JSON config
	serverURL := "http://localhost:9080"
	req, err := http.NewRequest("POST", serverURL+"/pyecho", requestBody)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Custom-Py-Header", "CaddyServerlessPythonTest")
	req.Header.Set("User-Agent", "Caddy-PyTest-Agent")

	client := &http.Client{Timeout: 120 * time.Second} // Further increased timeout
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status %d, got %d. Response body: %s", http.StatusOK, resp.StatusCode, string(bodyBytes))
	}

	responseBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	// Python's jsonify might return a map[string]interface{} for headers
	var pyEchoResp struct {
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}
	if err := json.Unmarshal(responseBodyBytes, &pyEchoResp); err != nil {
		t.Fatalf("Failed to unmarshal Python response JSON: %v. Body: %s", err, string(responseBodyBytes))
	}

	// Verify headers (case might differ with Flask, so check for presence and value)
	// Flask typically preserves original casing or Title-Cases them.
	// http.Header.Get() is case-insensitive for lookup.
	// The python app does `dict(request.headers)` which should preserve case as received by Flask.
	// Let's create an http.Header from the map for easier, case-insensitive checking.
	receivedHeaders := make(http.Header)
	for k, v := range pyEchoResp.Headers {
		receivedHeaders.Set(k, v)
	}

	if contentType := receivedHeaders.Get("Content-Type"); !strings.Contains(strings.ToLower(contentType), "application/json") {
		t.Errorf("Expected echoed 'Content-Type' header to contain 'application/json', got '%s'", contentType)
	}
	if customHeader := receivedHeaders.Get("X-Custom-Py-Header"); customHeader != "CaddyServerlessPythonTest" {
		t.Errorf("Expected echoed 'X-Custom-Py-Header' to be 'CaddyServerlessPythonTest', got '%s'", customHeader)
	}
	if userAgent := receivedHeaders.Get("User-Agent"); userAgent != "Caddy-PyTest-Agent" {
		t.Errorf("Expected echoed 'User-Agent' to be 'Caddy-PyTest-Agent', got '%s'", userAgent)
	}
	
	// Verify body
	if pyEchoResp.Body != requestPayload {
		t.Errorf("Expected Python echoed body to be '%s', got '%s'", requestPayload, pyEchoResp.Body)
	}

	t.Log("Serverless Python POST echo test completed successfully.")
}

// TestMain can be used for global setup/teardown if needed,
// for example, ensuring Docker is available.
func TestMain(m *testing.M) {
	// Optional: Check for Docker availability globally
	// if _, err := exec.LookPath("docker"); err != nil {
	// 	fmt.Println("SKIPPING serverless tests: Docker not found in PATH.")
	// 	os.Exit(0) // Skip all tests in this package
	// }
	os.Exit(m.Run())
}
