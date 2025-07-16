package serverless

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// TestSuite contains all integration tests for the serverless plugin
type TestSuite struct {
	t            *testing.T
	dockerClient *client.Client
	baseURL      string
	mu           sync.Mutex
}

// NewTestSuite creates a new test suite
func NewTestSuite(t *testing.T, baseURL string) (*TestSuite, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &TestSuite{
		t:            t,
		dockerClient: cli,
		baseURL:      baseURL,
	}, nil
}

// Helper function to make HTTP requests
func (ts *TestSuite) makeRequest(method, path string, body interface{}) (*http.Response, []byte, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, ts.baseURL+path, reqBody)
	if err != nil {
		return nil, nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, err
	}

	return resp, respBody, nil
}

// Helper function to check if a container is running
func (ts *TestSuite) isContainerRunning(imageName string) (bool, string) {
	ctx := context.Background()
	containers, err := ts.dockerClient.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		ts.t.Logf("Error listing containers: %v", err)
		return false, ""
	}

	for _, c := range containers {
		if strings.Contains(c.Image, imageName) {
			return true, c.ID
		}
	}
	return false, ""
}

// Helper function to wait for container to start
func (ts *TestSuite) waitForContainer(imageName string, timeout time.Duration) (string, error) {
	start := time.Now()
	for time.Since(start) < timeout {
		running, containerID := ts.isContainerRunning(imageName)
		if running {
			return containerID, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", fmt.Errorf("container %s did not start within %v", imageName, timeout)
}

// Container Launch Tests

func (ts *TestSuite) TestContainersNotRunningBeforeFirstRequest() {
	ts.t.Run("ContainersNotRunningBeforeFirstRequest", func(t *testing.T) {
		// Check that no test containers are running
		goRunning, _ := ts.isContainerRunning("caddy-serverless-go-echoserver-test")
		pythonRunning, _ := ts.isContainerRunning("caddy-serverless-py-echoserver-test")

		if goRunning {
			t.Error("Go echo server container is already running before first request")
		}
		if pythonRunning {
			t.Error("Python echo server container is already running before first request")
		}
	})
}

func (ts *TestSuite) TestGETRequestLaunchesGoContainer() {
	ts.t.Run("GETRequestLaunchesGoContainer", func(t *testing.T) {
		// Make GET request to Go endpoint
		resp, body, err := ts.makeRequest("GET", "/echo/go", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", resp.StatusCode, body)
		}

		// Check that container was launched
		containerID, err := ts.waitForContainer("caddy-serverless-go-echoserver-test", 5*time.Second)
		if err != nil {
			t.Errorf("Go container was not launched: %v", err)
		} else {
			t.Logf("Go container launched with ID: %s", containerID)
		}
	})
}

func (ts *TestSuite) TestPOSTRequestLaunchesPythonContainer() {
	ts.t.Run("POSTRequestLaunchesPythonContainer", func(t *testing.T) {
		// Make POST request to Python endpoint
		testData := map[string]string{
			"message": "Test from integration suite",
			"test":    "python-launch",
		}

		resp, body, err := ts.makeRequest("POST", "/echo/python", testData)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", resp.StatusCode, body)
		}

		// Check that container was launched
		containerID, err := ts.waitForContainer("caddy-serverless-py-echoserver-test", 5*time.Second)
		if err != nil {
			t.Errorf("Python container was not launched: %v", err)
		} else {
			t.Logf("Python container launched with ID: %s", containerID)
		}
	})
}

// Request/Response Tests

func (ts *TestSuite) TestGoEchoServerReturnsCorrectResponse() {
	ts.t.Run("GoEchoServerReturnsCorrectResponse", func(t *testing.T) {
		testData := map[string]interface{}{
			"message":   "Hello from Go test",
			"timestamp": time.Now().Format(time.RFC3339),
			"number":    42,
		}

		resp, body, err := ts.makeRequest("POST", "/echo/go", testData)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		// Parse response
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("Failed to parse response: %v. Body: %s", err, body)
		}

		// Check that response contains headers
		headers, ok := result["headers"].(map[string]interface{})
		if !ok {
			t.Error("Response does not contain headers")
		} else {
			if ct, ok := headers["Content-Type"]; ok {
				if !strings.Contains(fmt.Sprint(ct), "application/json") {
					t.Errorf("Content-Type header not passed through correctly: %v", ct)
				}
			}
		}

		// Check that response contains body
		respBody, ok := result["body"].(string)
		if !ok {
			t.Error("Response does not contain body")
		} else {
			// Verify the body matches what we sent
			var sentData map[string]interface{}
			if err := json.Unmarshal([]byte(respBody), &sentData); err != nil {
				t.Errorf("Failed to parse echoed body: %v", err)
			} else {
				if sentData["message"] != testData["message"] {
					t.Errorf("Echoed message doesn't match. Sent: %v, Got: %v", 
						testData["message"], sentData["message"])
				}
			}
		}
	})
}

func (ts *TestSuite) TestPythonEchoServerReturnsCorrectResponse() {
	ts.t.Run("PythonEchoServerReturnsCorrectResponse", func(t *testing.T) {
		testData := map[string]interface{}{
			"message": "Hello from Python test",
			"data": map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		}

		resp, body, err := ts.makeRequest("POST", "/echo/python", testData)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", resp.StatusCode, body)
		}

		// Parse response
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("Failed to parse response: %v. Body: %s", err, body)
		}

		// Check headers
		if headers, ok := result["headers"].(map[string]interface{}); ok {
			t.Logf("Received headers: %v", headers)
		} else {
			t.Error("Response does not contain headers")
		}

		// Check body
		if respBody, ok := result["body"].(string); ok {
			var sentData map[string]interface{}
			if err := json.Unmarshal([]byte(respBody), &sentData); err != nil {
				t.Errorf("Failed to parse echoed body: %v", err)
			} else {
				if sentData["message"] != testData["message"] {
					t.Errorf("Echoed message doesn't match")
				}
			}
		} else {
			t.Error("Response does not contain body")
		}
	})
}

// Container Lifecycle Tests

func (ts *TestSuite) TestContainerReuseForSubsequentRequests() {
	ts.t.Run("ContainerReuseForSubsequentRequests", func(t *testing.T) {
		// First request
		_, _, err := ts.makeRequest("POST", "/echo/go", map[string]string{"request": "1"})
		if err != nil {
			t.Fatalf("First request failed: %v", err)
		}

		// Wait for container to start and get its ID
		containerID1, err := ts.waitForContainer("caddy-serverless-go-echoserver-test", 5*time.Second)
		if err != nil {
			t.Fatalf("Container not found after first request: %v", err)
		}

		// Short delay to ensure container is fully ready
		time.Sleep(500 * time.Millisecond)

		// Second request
		_, _, err = ts.makeRequest("POST", "/echo/go", map[string]string{"request": "2"})
		if err != nil {
			t.Fatalf("Second request failed: %v", err)
		}

		// Check if same container is still running
		running, containerID2 := ts.isContainerRunning("caddy-serverless-go-echoserver-test")
		if !running {
			t.Error("Container is not running after second request")
		} else if containerID1 != containerID2 {
			t.Errorf("Different container ID after second request. First: %s, Second: %s", 
				containerID1, containerID2)
		} else {
			t.Log("Container was reused for subsequent request ✓")
		}
	})
}

// Error Handling Tests

func (ts *TestSuite) TestBehaviorWhenImageDoesNotExist() {
	ts.t.Run("BehaviorWhenImageDoesNotExist", func(t *testing.T) {
		// This test would require a special endpoint configured with a non-existent image
		// For now, we'll skip this as it requires modifying the Caddyfile
		t.Skip("Requires special configuration with non-existent image")
	})
}

func (ts *TestSuite) TestMethodRestrictions() {
	ts.t.Run("TestMethodRestrictions", func(t *testing.T) {
		// Python endpoint only accepts POST per the FastAPI implementation
		resp, body, err := ts.makeRequest("GET", "/echo/python", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}

		// FastAPI returns 405 Method Not Allowed for unsupported methods
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("Expected status 405 for GET on Python endpoint, got %d. Body: %s", 
				resp.StatusCode, body)
		}
	})
}

// Performance Tests

func (ts *TestSuite) TestConcurrentRequestsToSameEndpoint() {
	ts.t.Run("ConcurrentRequestsToSameEndpoint", func(t *testing.T) {
		var wg sync.WaitGroup
		errors := make([]error, 0)
		errorsMu := sync.Mutex{}

		// Make 10 concurrent requests
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(reqNum int) {
				defer wg.Done()
				
				data := map[string]interface{}{
					"request": reqNum,
					"timestamp": time.Now().Format(time.RFC3339Nano),
				}
				
				resp, body, err := ts.makeRequest("POST", "/echo/go", data)
				if err != nil {
					errorsMu.Lock()
					errors = append(errors, fmt.Errorf("request %d failed: %w", reqNum, err))
					errorsMu.Unlock()
					return
				}
				
				if resp.StatusCode != http.StatusOK {
					errorsMu.Lock()
					errors = append(errors, fmt.Errorf("request %d got status %d: %s", 
						reqNum, resp.StatusCode, body))
					errorsMu.Unlock()
				}
			}(i)
		}

		wg.Wait()

		if len(errors) > 0 {
			t.Errorf("Concurrent requests had %d errors:", len(errors))
			for _, err := range errors {
				t.Errorf("  - %v", err)
			}
		} else {
			t.Log("All concurrent requests completed successfully ✓")
		}

		// Verify only one container is running
		time.Sleep(500 * time.Millisecond)
		ctx := context.Background()
		containers, _ := ts.dockerClient.ContainerList(ctx, container.ListOptions{})
		
		goContainerCount := 0
		for _, c := range containers {
			if strings.Contains(c.Image, "caddy-serverless-go-echoserver-test") {
				goContainerCount++
			}
		}
		
		if goContainerCount > 1 {
			t.Errorf("Expected 1 Go container, found %d", goContainerCount)
		}
	})
}

func (ts *TestSuite) TestConcurrentRequestsToDifferentEndpoints() {
	ts.t.Run("ConcurrentRequestsToDifferentEndpoints", func(t *testing.T) {
		var wg sync.WaitGroup
		
		// Make concurrent requests to both endpoints
		for i := 0; i < 5; i++ {
			wg.Add(2)
			
			// Request to Go endpoint
			go func(reqNum int) {
				defer wg.Done()
				data := map[string]interface{}{"endpoint": "go", "request": reqNum}
				resp, _, err := ts.makeRequest("POST", "/echo/go", data)
				if err != nil {
					t.Errorf("Go request %d failed: %v", reqNum, err)
				} else if resp.StatusCode != http.StatusOK {
					t.Errorf("Go request %d got status %d", reqNum, resp.StatusCode)
				}
			}(i)
			
			// Request to Python endpoint
			go func(reqNum int) {
				defer wg.Done()
				data := map[string]interface{}{"endpoint": "python", "request": reqNum}
				resp, _, err := ts.makeRequest("POST", "/echo/python", data)
				if err != nil {
					t.Errorf("Python request %d failed: %v", reqNum, err)
				} else if resp.StatusCode != http.StatusOK {
					t.Errorf("Python request %d got status %d", reqNum, resp.StatusCode)
				}
			}(i)
		}
		
		wg.Wait()
		
		// Verify both containers are running
		time.Sleep(1 * time.Second)
		goRunning, _ := ts.isContainerRunning("caddy-serverless-go-echoserver-test")
		pythonRunning, _ := ts.isContainerRunning("caddy-serverless-py-echoserver-test")
		
		if !goRunning {
			t.Error("Go container is not running after concurrent requests")
		}
		if !pythonRunning {
			t.Error("Python container is not running after concurrent requests")
		}
		
		t.Log("Both containers running after concurrent requests ✓")
	})
}

// Helper function to measure request time
func (ts *TestSuite) measureRequestTime(method, path string, body interface{}) (time.Duration, error) {
	start := time.Now()
	resp, _, err := ts.makeRequest(method, path, body)
	duration := time.Since(start)
	
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return duration, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	
	return duration, nil
}

func (ts *TestSuite) TestColdStartVsWarmRequests() {
	ts.t.Run("ColdStartVsWarmRequests", func(t *testing.T) {
		// Ensure container is not running
		ctx := context.Background()
		containers, _ := ts.dockerClient.ContainerList(ctx, container.ListOptions{})
		for _, c := range containers {
			if strings.Contains(c.Image, "caddy-serverless-go-echoserver-test") {
				ts.dockerClient.ContainerStop(ctx, c.ID, container.StopOptions{})
				ts.dockerClient.ContainerRemove(ctx, c.ID, types.ContainerRemoveOptions{})
			}
		}
		
		time.Sleep(2 * time.Second)
		
		// Cold start request
		coldDuration, err := ts.measureRequestTime("POST", "/echo/go", map[string]string{"type": "cold"})
		if err != nil {
			t.Fatalf("Cold start request failed: %v", err)
		}
		t.Logf("Cold start time: %v", coldDuration)
		
		// Wait a bit to ensure container is fully ready
		time.Sleep(500 * time.Millisecond)
		
		// Warm requests
		var warmDurations []time.Duration
		for i := 0; i < 5; i++ {
			warmDuration, err := ts.measureRequestTime("POST", "/echo/go", map[string]string{"type": "warm", "iteration": fmt.Sprint(i)})
			if err != nil {
				t.Errorf("Warm request %d failed: %v", i, err)
				continue
			}
			warmDurations = append(warmDurations, warmDuration)
		}
		
		// Calculate average warm time
		var totalWarm time.Duration
		for _, d := range warmDurations {
			totalWarm += d
		}
		avgWarm := totalWarm / time.Duration(len(warmDurations))
		
		t.Logf("Average warm request time: %v", avgWarm)
		t.Logf("Cold start overhead: %v", coldDuration-avgWarm)
		
		// Cold start should be significantly slower than warm requests
		if coldDuration < avgWarm*2 {
			t.Log("Warning: Cold start not significantly slower than warm requests")
		}
	})
}

// RunAllTests executes all test cases
func (ts *TestSuite) RunAllTests() {
	// Container Launch Tests
	ts.TestContainersNotRunningBeforeFirstRequest()
	ts.TestGETRequestLaunchesGoContainer()
	ts.TestPOSTRequestLaunchesPythonContainer()
	
	// Request/Response Tests
	ts.TestGoEchoServerReturnsCorrectResponse()
	ts.TestPythonEchoServerReturnsCorrectResponse()
	
	// Container Lifecycle Tests
	ts.TestContainerReuseForSubsequentRequests()
	
	// Error Handling Tests
	ts.TestMethodRestrictions()
	
	// Performance Tests
	ts.TestConcurrentRequestsToSameEndpoint()
	ts.TestConcurrentRequestsToDifferentEndpoints()
	ts.TestColdStartVsWarmRequests()
}