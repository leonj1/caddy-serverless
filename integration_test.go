// Copyright 2015 Matthew Holt and The Caddy Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package serverless

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

// fakeRequest is a helper to create mock HTTP requests for testing
func fakeRequest(method, path string) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	// Add a context that won't be canceled prematurely, similar to what caddyhttp.Context would provide
	// This helps avoid "context canceled" errors in tests if the test finishes before the handler.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute) // Generous timeout for test
	// Normally, you'd use defer cancel(), but since the request's lifetime is tied to the test,
	// and we're just providing a context, this is okay. The test's own context management (if any)
	// or the handler's context management will take precedence.
	// For more complex scenarios, ensure proper context cancellation.
	_ = cancel // Avoid unused variable error if not immediately used.
	return r.WithContext(ctx)
}

// MockRoundTripper is a custom http.RoundTripper for mocking HTTP responses
type MockRoundTripper struct {
	Response    *http.Response
	Error       error
	RequestFunc func(req *http.Request) // Optional: to inspect the request
}

// RoundTrip implements the http.RoundTripper interface
func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.RequestFunc != nil {
		m.RequestFunc(req)
	}
	if m.Error != nil {
		return nil, m.Error
	}
	return m.Response, nil
}

// MockContainerManager is a mock implementation for testing
type MockContainerManager struct {
	startContainerFn  func(ctx context.Context, config ContainerConfig) (*Container, error)
	containers        map[string]*Container
	shouldFail        bool
}

func NewMockContainerManager() *MockContainerManager {
	m := &MockContainerManager{
		containers: make(map[string]*Container),
	}

	// Set default StartContainer implementation
	m.startContainerFn = func(_ context.Context, config ContainerConfig) (*Container, error) {
		if m.shouldFail {
			return nil, &MockError{message: "mock container start failure"}
		}

		container := &Container{
			ID:   "mock-container-id",
			IP:   "127.0.0.1",
			Port: 8080,
		}

		m.containers[container.ID] = container
		return container, nil
	}

	return m
}

// StartContainer implements ContainerManagerInterface by calling the function field
func (m *MockContainerManager) StartContainer(ctx context.Context, config ContainerConfig) (*Container, error) {
	return m.startContainerFn(ctx, config)
}

// SetStartContainerFunc allows overriding the StartContainer behavior
func (m *MockContainerManager) SetStartContainerFunc(fn func(ctx context.Context, config ContainerConfig) (*Container, error)) {
	m.startContainerFn = fn
}

// Ensure MockContainerManager implements ContainerManagerInterface
var _ ContainerManagerInterface = (*MockContainerManager)(nil)

func (m *MockContainerManager) WaitForReady(_ context.Context, container *Container, timeout time.Duration, port int) error {
	if m.shouldFail {
		return &MockError{message: "mock container not ready"}
	}
	return nil
}

func (m *MockContainerManager) StopContainer(_ context.Context, containerID string) error {
	delete(m.containers, containerID)
	return nil
}

func (m *MockContainerManager) Cleanup() error {
	m.containers = make(map[string]*Container)
	return nil
}

type MockError struct {
	message string
}

func (e *MockError) Error() string {
	return e.message
}

// TestHandler_Integration tests the complete flow with mocked Docker
func TestHandler_Integration(t *testing.T) {
	// Create handler with mock container manager
	handler := &Handler{
		Functions: []FunctionConfig{
			{
				Methods:   []string{"GET", "POST"},
				Path:      "/api/test.*",
				Image:     "test:latest",
				Port:      8080,
				Timeout:   caddy.Duration(30 * time.Second),
				pathRegex: regexp.MustCompile("/api/test.*"),
			},
		},
	}

	// Provision the handler to initialize logger
	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	defer cancel()
	err := handler.Provision(ctx)
	if err != nil {
		t.Fatalf("failed to provision handler: %v", err)
	}

	// Replace the container manager with a mock
	mockCM := NewMockContainerManager()
	handler.containerManager = mockCM

	// Create a mock HTTP client
	mockRT := &MockRoundTripper{
		Response: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("mocked response")),
			Header:     make(http.Header),
		},
	}
	handler.HTTPClient = &http.Client{Transport: mockRT}

	// Create a test request
	req := fakeRequest("GET", "/api/test/123")
	w := httptest.NewRecorder()

	// Mock next handler (should not be called)
	nextCalled := false
	next := caddyhttp.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) error {
		nextCalled = true
		return nil
	})

	// Execute the handler
	err = handler.ServeHTTP(w, req, next)

	// We expect no error now that the proxy is mocked
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if nextCalled {
		t.Error("next handler should not have been called")
	}

	// Check the response
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	expectedBody := "mocked response"
	if w.Body.String() != expectedBody {
		t.Errorf("expected body '%s', got '%s'", expectedBody, w.Body.String())
	}

	// Check that container was started and stopped
	if len(mockCM.containers) != 0 {
		t.Errorf("expected containers to be cleaned up, but %d remain", len(mockCM.containers))
	}
}

// TestHandler_FullProxyIntegration tests the complete proxy flow with a real backend server
func TestHandler_FullProxyIntegration(t *testing.T) {
	// Start a test HTTP server to act as the backend container
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo back request information to verify proxy functionality
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Backend-Server", "test-container")
		w.WriteHeader(http.StatusOK)
		
		response := map[string]interface{}{
			"method":      r.Method,
			"path":        r.URL.Path,
			"query":       r.URL.RawQuery,
			"headers":     r.Header,
			"remote_addr": r.RemoteAddr,
		}
		
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer backendServer.Close()

	// Parse the backend server URL to get host and port
	backendURL := backendServer.URL
	parts := strings.Split(strings.TrimPrefix(backendURL, "http://"), ":")
	if len(parts) != 2 {
		t.Fatalf("unexpected backend server URL format: %s", backendURL)
	}
	backendHost := parts[0]
	backendPort := parts[1]
	backendPortInt := 0
	if port, err := json.Number(backendPort).Int64(); err == nil {
		backendPortInt = int(port)
	} else {
		t.Fatalf("failed to parse backend port: %v", err)
	}

	// Create a mock container manager that returns the backend server details
	mockCM := NewMockContainerManager()

	// Override StartContainer to return a container pointing to our test server
	originalStartContainer := mockCM.startContainerFn
	mockCM.SetStartContainerFunc(func(_ context.Context, config ContainerConfig) (*Container, error) {
		container := &Container{
			ID:   "test-container-id",
			IP:   backendHost,
			Port: backendPortInt,
		}
		mockCM.containers[container.ID] = container
		return container, nil
	})

	// Create handler with the mock container manager
	handler := &Handler{
		Functions: []FunctionConfig{
			{
				Methods:   []string{"GET", "POST"},
				Path:      "/api/function.*",
				Image:     "test:latest",
				Port:      backendPortInt, // Use the actual port of the mock backend server
				Timeout:   caddy.Duration(30 * time.Second),
				pathRegex: regexp.MustCompile("/api/function.*"),
			},
		},
	}

	// Provision the handler
	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	defer cancel()
	err := handler.Provision(ctx)
	if err != nil {
		t.Fatalf("failed to provision handler: %v", err)
	}

	// Replace the container manager with our mock
	handler.containerManager = mockCM

	// Test GET request
	t.Run("GET request with query parameters", func(t *testing.T) {
		req := fakeRequest("GET", "/api/function/test?param1=value1&param2=value2")
		req.Header.Set("X-Test-Header", "test-value")
		req.Header.Set("User-Agent", "test-agent")
		w := httptest.NewRecorder()

		next := caddyhttp.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) error {
			t.Error("next handler should not be called")
			return nil
		})

		err := handler.ServeHTTP(w, req, next)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify response status
		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		// Verify backend server header was set
		if w.Header().Get("X-Backend-Server") != "test-container" {
			t.Errorf("expected X-Backend-Server header to be 'test-container', got '%s'", w.Header().Get("X-Backend-Server"))
		}

		// Parse and verify response body
		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to parse response JSON: %v", err)
		}

		// Verify request details were proxied correctly
		if response["method"] != "GET" {
			t.Errorf("expected method 'GET', got '%v'", response["method"])
		}
		if response["path"] != "/api/function/test" {
			t.Errorf("expected path '/api/function/test', got '%v'", response["path"])
		}
		if response["query"] != "param1=value1&param2=value2" {
			t.Errorf("expected query 'param1=value1&param2=value2', got '%v'", response["query"])
		}

		// Verify headers were proxied
		headers, ok := response["headers"].(map[string]interface{})
		if !ok {
			t.Fatal("headers not found in response")
		}
		
		// Check that our custom header was proxied
		testHeader, exists := headers["X-Test-Header"]
		if !exists {
			t.Error("X-Test-Header was not proxied to backend")
		} else if headerSlice, ok := testHeader.([]interface{}); ok && len(headerSlice) > 0 {
			if headerSlice[0] != "test-value" {
				t.Errorf("expected X-Test-Header value 'test-value', got '%v'", headerSlice[0])
			}
		}
	})

	// Test POST request with body
	t.Run("POST request with JSON body", func(t *testing.T) {
		requestBody := `{"key": "value", "number": 42}`
		req := httptest.NewRequest("POST", "/api/function/submit", strings.NewReader(requestBody))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(context.Background())
		w := httptest.NewRecorder()

		next := caddyhttp.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) error {
			t.Error("next handler should not be called")
			return nil
		})

		err := handler.ServeHTTP(w, req, next)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify response
		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to parse response JSON: %v", err)
		}

		if response["method"] != "POST" {
			t.Errorf("expected method 'POST', got '%v'", response["method"])
		}
		if response["path"] != "/api/function/submit" {
			t.Errorf("expected path '/api/function/submit', got '%v'", response["path"])
		}

		// Verify Content-Type header was proxied
		headers, ok := response["headers"].(map[string]interface{})
		if !ok {
			t.Fatal("headers not found in response")
		}
		
		contentType, exists := headers["Content-Type"]
		if !exists {
			t.Error("Content-Type header was not proxied to backend")
		} else if headerSlice, ok := contentType.([]interface{}); ok && len(headerSlice) > 0 {
			if headerSlice[0] != "application/json" {
				t.Errorf("expected Content-Type 'application/json', got '%v'", headerSlice[0])
			}
		}
	})

	// Verify container cleanup
	if len(mockCM.containers) != 0 {
		t.Errorf("expected containers to be cleaned up, but %d remain", len(mockCM.containers))
	}

	// Restore original StartContainer method
	mockCM.startContainerFn = originalStartContainer
}

// TestHandler_Integration_ProxyFailure tests the proxy failure path
func TestHandler_Integration_ProxyFailure(t *testing.T) {
	// Create handler with mock container manager
	handler := &Handler{
		Functions: []FunctionConfig{
			{
				Methods:   []string{"GET"},
				Path:      "/api/proxyfail",
				Image:     "test:latest",
				Port:      8080,
				Timeout:   caddy.Duration(30 * time.Second),
				pathRegex: regexp.MustCompile("/api/proxyfail"),
			},
		},
	}

	// Provision the handler
	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	defer cancel()
	if err := handler.Provision(ctx); err != nil {
		t.Fatalf("failed to provision handler: %v", err)
	}

	// Replace the container manager with a mock
	mockCM := NewMockContainerManager()
	handler.containerManager = mockCM

	// Create a mock HTTP client that returns an error
	mockRT := &MockRoundTripper{
		Error: fmt.Errorf("mock proxy error"),
	}
	handler.HTTPClient = &http.Client{Transport: mockRT}

	// Create a test request
	req := fakeRequest("GET", "/api/proxyfail")
	w := httptest.NewRecorder()
	next := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error { return nil })

	// Execute the handler
	err := handler.ServeHTTP(w, req, next)

	// We expect an error because the mocked proxy call fails
	if err == nil {
		t.Error("expected error when proxying fails")
	} else {
		if herr, ok := err.(caddyhttp.HandlerError); ok {
			if herr.StatusCode != http.StatusBadGateway {
				t.Errorf("expected status %d for proxy error, got %d", http.StatusBadGateway, herr.StatusCode)
			}
		} else {
			t.Errorf("expected HandlerError, got %T: %v", err, err)
		}
	}

	// Check that container was started and stopped
	if len(mockCM.containers) != 0 {
		t.Errorf("expected containers to be cleaned up, but %d remain", len(mockCM.containers))
	}
}

// TestHandler_NoMatchPassesToNext tests that unmatched requests pass to next handler
func TestHandler_NoMatchPassesToNext(t *testing.T) {
	handler := &Handler{
		Functions: []FunctionConfig{
			{
				Methods:   []string{"GET"},
				Path:      "/api/test",
				Image:     "test:latest",
				pathRegex: regexp.MustCompile("/api/test"),
			},
		},
	}
	// Provision the handler to initialize logger and regexes
	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	defer cancel()
	errProvision := handler.Provision(ctx)
	if errProvision != nil {
		t.Fatalf("failed to provision handler: %v", errProvision)
	}


	req := fakeRequest("POST", "/other/path")
	w := httptest.NewRecorder()

	nextCalled := false
	next := caddyhttp.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) error {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
		return nil
	})

	err := handler.ServeHTTP(w, req, next)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !nextCalled {
		t.Error("next handler should have been called")
	}

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// TestHandler_ContainerStartFailure tests error handling when container fails to start
func TestHandler_ContainerStartFailure(t *testing.T) {
	handler := &Handler{
		Functions: []FunctionConfig{
			{
				Methods:   []string{"GET"},
				Path:      "/api/test",
				Image:     "test:latest",
				pathRegex: regexp.MustCompile("/api/test"),
			},
		},
	}

	// Provision the handler to initialize logger
	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	defer cancel()
	err := handler.Provision(ctx)
	if err != nil {
		t.Fatalf("failed to provision handler: %v", err)
	}

	// Use a mock container manager that fails
	mockCM := NewMockContainerManager()
	mockCM.shouldFail = true
	handler.containerManager = mockCM

	req := fakeRequest("GET", "/api/test")
	w := httptest.NewRecorder()

	next := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})

	err = handler.ServeHTTP(w, req, next)

	// Should return an error
	if err == nil {
		t.Error("expected error when container fails to start")
	}

	// Should be a HandlerError with 500 status
	if handlerErr, ok := err.(caddyhttp.HandlerError); ok {
		if handlerErr.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected status 500, got %d", handlerErr.StatusCode)
		}
	} else {
		t.Error("expected HandlerError")
	}
}

// TestHandler_JSONConfiguration tests JSON configuration parsing
func TestHandler_JSONConfiguration(t *testing.T) {
	jsonConfig := `{
		"functions": [
			{
				"methods": ["GET", "POST"],
				"path": "/api/.*",
				"image": "nginx:latest",
				"command": ["/bin/sh", "-c", "echo hello"],
				"environment": {
					"KEY": "value"
				},
				"volumes": [
					{
						"source": "/host/path",
						"target": "/container/path",
						"readonly": true
					}
				],
				"timeout": "30s",
				"port": 8080
			}
		]
	}`

	var handler Handler
	err := json.Unmarshal([]byte(jsonConfig), &handler)
	if err != nil {
		t.Fatalf("failed to unmarshal JSON config: %v", err)
	}

	if len(handler.Functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(handler.Functions))
	}

	fn := handler.Functions[0]

	// Verify configuration
	expectedMethods := []string{"GET", "POST"}
	if len(fn.Methods) != len(expectedMethods) {
		t.Errorf("expected %d methods, got %d", len(expectedMethods), len(fn.Methods))
	}

	if fn.Path != "/api/.*" {
		t.Errorf("expected path '/api/.*', got '%s'", fn.Path)
	}

	if fn.Image != "nginx:latest" {
		t.Errorf("expected image 'nginx:latest', got '%s'", fn.Image)
	}

	expectedCommand := []string{"/bin/sh", "-c", "echo hello"}
	if len(fn.Command) != len(expectedCommand) {
		t.Errorf("expected %d command args, got %d", len(expectedCommand), len(fn.Command))
	}

	if fn.Environment["KEY"] != "value" {
		t.Errorf("expected env KEY=value, got '%s'", fn.Environment["KEY"])
	}

	if len(fn.Volumes) != 1 {
		t.Errorf("expected 1 volume, got %d", len(fn.Volumes))
	}

	vol := fn.Volumes[0]
	if vol.Source != "/host/path" {
		t.Errorf("expected volume source '/host/path', got '%s'", vol.Source)
	}

	if vol.Target != "/container/path" {
		t.Errorf("expected volume target '/container/path', got '%s'", vol.Target)
	}

	if !vol.ReadOnly {
		t.Error("expected volume to be read-only")
	}

	if fn.Timeout != caddy.Duration(30*time.Second) {
		t.Errorf("expected timeout 30s, got %v", fn.Timeout)
	}

	if fn.Port != 8080 {
		t.Errorf("expected port 8080, got %d", fn.Port)
	}
}

// TestHandler_Cleanup tests the cleanup functionality
func TestHandler_Cleanup(t *testing.T) {
	handler := &Handler{}
	mockCM := NewMockContainerManager()
	handler.containerManager = mockCM

	// Add some mock containers
	mockCM.containers["container1"] = &Container{ID: "container1"}
	mockCM.containers["container2"] = &Container{ID: "container2"}

	err := handler.Cleanup()
	if err != nil {
		t.Errorf("unexpected error during cleanup: %v", err)
	}

	if len(mockCM.containers) != 0 {
		t.Errorf("expected all containers to be cleaned up, but %d remain", len(mockCM.containers))
	}
}

// TestHandler_MethodCaseInsensitive tests case-insensitive method matching
func TestHandler_MethodCaseInsensitive(t *testing.T) {
	handler := &Handler{
		Functions: []FunctionConfig{
			{
				Methods: []string{"GET", "post"},
				Path:    "/test",
				Image:   "test:latest",
				// pathRegex will be compiled during Provision
			},
		},
	}

	// Provision the handler to compile regexes and initialize routeMap
	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	defer cancel()
	errProvision := handler.Provision(ctx)
	if errProvision != nil {
		t.Fatalf("failed to provision handler: %v", errProvision)
	}


	tests := []struct {
		method      string
		shouldMatch bool
	}{
		{"GET", true},
		{"get", true},
		{"POST", true},
		{"post", true},
		{"PUT", false},
		{"put", false},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			req := fakeRequest(tt.method, "/test")
			result := handler.findMatchingFunction(req)

			if tt.shouldMatch && result == nil {
				t.Error("expected to find matching function")
			} else if !tt.shouldMatch && result != nil {
				t.Error("expected no matching function")
			}
		})
	}
}
