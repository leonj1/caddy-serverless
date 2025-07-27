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
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func init() {
	caddy.RegisterModule(ServerlessHandler{})
}

// ServerlessHandler implements a serverless functions handler that executes
// Docker containers in response to HTTP requests based on configured routes.
type ServerlessHandler struct {
	// Functions defines the serverless function configurations
	Functions []FunctionConfig `json:"functions,omitempty"`

	// HTTPClient is the client used to make requests to containers.
	// It can be overridden for testing.
	HTTPClient *http.Client `json:"-"`

	logger           *zap.Logger
	containerManager ContainerManagerInterface
	routeMap         methodMap
}

// methodMap stores a map of HTTP methods to a map of path regexes to function configurations.
type methodMap map[string]map[*regexp.Regexp]*FunctionConfig

// FunctionConfig represents the configuration for a single serverless function
type FunctionConfig struct {
	// Methods specifies the HTTP methods this function handles (GET, POST, PUT, DELETE, etc.)
	Methods []string `json:"methods,omitempty"`

	// Path specifies the URL path pattern this function handles (supports regex)
	Path string `json:"path,omitempty"`

	// Image specifies the Docker image to run
	Image string `json:"image,omitempty"`

	// Command specifies the command to run in the container
	Command []string `json:"command,omitempty"`

	// Environment specifies environment variables to pass to the container
	Environment map[string]string `json:"environment,omitempty"`

	// Volumes specifies volume mounts for the container
	Volumes []VolumeMount `json:"volumes,omitempty"`

	// Timeout specifies the maximum execution time for the function
	Timeout caddy.Duration `json:"timeout,omitempty"`

	// Port specifies the port the container listens on (default: 8080)
	Port int `json:"port,omitempty"`

	// compiled regex for path matching
	pathRegex *regexp.Regexp
}

// CaddyModule returns the Caddy module information.
func (ServerlessHandler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.serverless",
		New: func() caddy.Module { return new(ServerlessHandler) },
	}
}

// Provision sets up the serverless handler.
func (h *ServerlessHandler) Provision(ctx caddy.Context) error {
	h.logger = ctx.Logger()
	if h.HTTPClient == nil {
		h.HTTPClient = &http.Client{
			Timeout: 30 * time.Second, // Default timeout
		}
	}
	h.containerManager = NewContainerManager(h.logger)
	h.routeMap = make(methodMap)

	// Compile regex patterns for path matching and populate routeMap
	for i := range h.Functions {
		fn := &h.Functions[i] // Use a pointer to modify the original slice element

		if fn.Path != "" {
			regex, err := regexp.Compile(fn.Path)
			if err != nil {
				return fmt.Errorf("invalid path regex for function %d: %v", i, err)
			}
			fn.pathRegex = regex
		} else {
			return fmt.Errorf("function %d: path is required", i)
		}

		// Set default port if not specified
		if fn.Port == 0 {
			fn.Port = 8080
		}

		// Set default timeout if not specified
		if fn.Timeout == 0 {
			fn.Timeout = caddy.Duration(30 * time.Second)
		}

		// Validate required fields
		if fn.Image == "" {
			return fmt.Errorf("function %d: image is required", i)
		}
		if len(fn.Methods) == 0 {
			return fmt.Errorf("function %d: at least one method is required", i)
		}

		// Populate the routeMap
		for _, method := range fn.Methods {
			upperMethod := strings.ToUpper(method)
			if h.routeMap[upperMethod] == nil {
				h.routeMap[upperMethod] = make(map[*regexp.Regexp]*FunctionConfig)
			}
			h.routeMap[upperMethod][fn.pathRegex] = fn
		}
	}

	return nil
}

// Validate ensures the configuration is valid.
func (h ServerlessHandler) Validate() error {
	for i, fn := range h.Functions {
		// Validate methods
		for _, method := range fn.Methods {
			switch strings.ToUpper(method) {
			case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS":
				// Valid methods
			default:
				return fmt.Errorf("function %d: invalid HTTP method '%s'", i, method)
			}
		}

		// Validate volume mounts
		for j, vol := range fn.Volumes {
			if vol.Source == "" {
				return fmt.Errorf("function %d, volume %d: source path is required", i, j)
			}
			if vol.Target == "" {
				return fmt.Errorf("function %d, volume %d: target path is required", i, j)
			}
			if !filepath.IsAbs(vol.Source) {
				return fmt.Errorf("function %d, volume %d: source path must be absolute", i, j)
			}
			if !filepath.IsAbs(vol.Target) {
				return fmt.Errorf("function %d, volume %d: target path must be absolute", i, j)
			}
		}
	}

	return nil
}

// ServeHTTP implements the HTTP handler interface.
func (h ServerlessHandler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	// Find matching function
	function := h.findMatchingFunction(r)
	if function == nil {
		// No matching function, pass to next handler
		return next.ServeHTTP(w, r)
	}

	h.logger.Debug("executing serverless function",
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path),
		zap.String("image", function.Image))

	// Execute the function
	return h.executeFunction(w, r, function)
}

// findMatchingFunction finds the first function that matches the request
func (h *ServerlessHandler) findMatchingFunction(r *http.Request) *FunctionConfig {
	requestMethod := strings.ToUpper(r.Method)
	pathMap, methodExists := h.routeMap[requestMethod]

	if !methodExists {
		return nil
	}

	for pathRegex, function := range pathMap {
		if pathRegex != nil && pathRegex.MatchString(r.URL.Path) {
			return function
		}
	}

	return nil
}

// executeFunction executes a serverless function in a Docker container
func (h *ServerlessHandler) executeFunction(w http.ResponseWriter, r *http.Request, function *FunctionConfig) error {
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(function.Timeout))
	defer cancel()

	// Create a separate context for container lifecycle operations to ensure cleanup
	// operations are not affected by request context cancellation or timeout
	lifecycleCtx := context.Background()

	// Prepare container configuration
	config := ContainerConfig{
		Image:       function.Image,
		Command:     function.Command,
		Environment: function.Environment,
		Volumes:     function.Volumes,
		Port:        function.Port,
	}

	// Start container
	container, err := h.containerManager.StartContainer(ctx, config)
	if err != nil {
		h.logger.Error("failed to start container",
			zap.Error(err),
			zap.String("image", config.Image),
			zap.Int("port", config.Port),
			zap.Duration("timeout", time.Duration(function.Timeout)))
		return caddyhttp.Error(http.StatusInternalServerError, err)
	}

	// Ensure container cleanup using lifecycle context to prevent cleanup failures
	// due to request context cancellation or timeout
	defer func() {
		if err := h.containerManager.StopContainer(lifecycleCtx, container.ID); err != nil {
			h.logger.Error("failed to stop container", zap.String("container_id", container.ID), zap.Error(err))
		}
	}()

	// Wait for container to be ready
	if err := h.containerManager.WaitForReady(ctx, container, time.Duration(function.Timeout), function.Port); err != nil {
		h.logger.Error("container failed to become ready", zap.Error(err))
		return caddyhttp.Error(http.StatusInternalServerError, err)
	}

	// Proxy request to container
	return h.proxyToContainer(w, r, container, function.Port)
}

// proxyToContainer proxies the HTTP request to the running container
func (h *ServerlessHandler) proxyToContainer(w http.ResponseWriter, r *http.Request, container *Container, internalAppPort int) error {
	// Create request to container
	// Use container.IP (internal IP) and internalAppPort (the port the app inside the container listens on)
	containerURL := fmt.Sprintf("http://%s:%d%s", container.IP, internalAppPort, r.URL.Path)
	if r.URL.RawQuery != "" {
		containerURL += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, containerURL, r.Body)
	if err != nil {
		return caddyhttp.Error(http.StatusInternalServerError, err)
	}

	// Copy headers
	for name, values := range r.Header {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	// Make request to container
	resp, err := h.HTTPClient.Do(req)
	if err != nil {
		h.logger.Error("failed to proxy request to container", zap.Error(err))
		return caddyhttp.Error(http.StatusBadGateway, err)
	}
	defer resp.Body.Close()

	// Copy response headers
	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	// Copy status code
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		h.logger.Error("failed to copy response body", zap.Error(err))
		return err
	}

	return nil
}

// Cleanup cleans up resources when the handler is being shut down.
func (h *ServerlessHandler) Cleanup() error {
	if h.containerManager != nil {
		return h.containerManager.Cleanup()
	}
	return nil
}

// Interface guards
var (
	_ caddy.Provisioner           = (*ServerlessHandler)(nil)
	_ caddy.Validator             = (*ServerlessHandler)(nil)
	_ caddy.CleanerUpper          = (*ServerlessHandler)(nil)
	_ caddyhttp.MiddlewareHandler = (*ServerlessHandler)(nil)
)
