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
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ContainerManagerInterface defines the abstraction for managing Docker containers,
// allowing for easier testing and potential support for alternative container runtimes.
type ContainerManagerInterface interface {
	StartContainer(ctx context.Context, config ContainerConfig) (*Container, error)
	WaitForReady(ctx context.Context, container *Container, timeout time.Duration, port int) error
	StopContainer(ctx context.Context, containerID string) error
	Cleanup() error
}

// ContainerManager manages Docker containers for serverless functions
type ContainerManager struct {
	logger     *zap.Logger
	containers map[string]*Container
	mutex      sync.RWMutex
	httpClient *http.Client
}

// Container represents a running Docker container
type Container struct {
	ID   string
	IP   string
	Port int
}

// VolumeMount represents a Docker volume mount
type VolumeMount struct {
	Source   string
	Target   string
	ReadOnly bool
}

// ContainerConfig represents the configuration for starting a container
type ContainerConfig struct {
	Image       string
	Command     []string
	Environment map[string]string
	Volumes     []VolumeMount
	Port        int
}

// validateDockerImage checks if the Docker image name is valid.
// Basic validation: non-empty. More sophisticated validation can be added,
// e.g., regex for valid image names from Docker's spec.
func validateDockerImage(image string) bool {
	// For now, just ensure it's not empty.
	// A more robust check might involve parsing image name components (name, tag, digest).
	return strings.TrimSpace(image) != ""
}

// validateDockerCommand checks if a Docker command part is valid.
// Basic validation: non-empty.
func validateDockerCommand(command string) bool {
	// For now, just ensure it's not empty.
	// Further validation could check for disallowed characters or patterns,
	// but this can be complex and depends on the execution context.
	return strings.TrimSpace(command) != ""
}

// validateContainerConfig validates the container configuration fields.
func validateContainerConfig(config ContainerConfig) error {
	if !validateDockerImage(config.Image) {
		return fmt.Errorf("invalid docker image name: '%s'", config.Image)
	}
	if len(config.Command) > 0 {
		for i, cmdPart := range config.Command {
			if !validateDockerCommand(cmdPart) {
				return fmt.Errorf("invalid docker command part at index %d: '%s'", i, cmdPart)
			}
		}
	}

	// Validate Environment variables
	for key, _ := range config.Environment {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("environment variable key cannot be empty")
		}
		// Potentially add more checks for key/value formats if needed
	}

	// Validate Volumes
	for i, volume := range config.Volumes {
		if strings.TrimSpace(volume.Source) == "" {
			return fmt.Errorf("volume mount source cannot be empty at index %d", i)
		}
		if strings.TrimSpace(volume.Target) == "" {
			return fmt.Errorf("volume mount target cannot be empty at index %d", i)
		}
		// Potentially add more checks for path validity
	}
	return nil
}

// NewContainerManager creates a new container manager
func NewContainerManager(logger *zap.Logger) *ContainerManager {
	return &ContainerManager{
		logger:     logger,
		containers: make(map[string]*Container),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression: true,
			},
		},
	}
}

// StartContainer starts a new Docker container with the given configuration
func (cm *ContainerManager) StartContainer(ctx context.Context, config ContainerConfig) (*Container, error) {
	// Validate container configuration
	if err := validateContainerConfig(config); err != nil {
		return nil, fmt.Errorf("invalid container configuration: %w", err)
	}

	// Build docker run command
	args := []string{"run", "-d", "--rm"}

	// Use host networking mode
	args = append(args, "--network", "host")

	// Add environment variables
	for key, value := range config.Environment {
		args = append(args, "-e", fmt.Sprintf("%s=%s", key, value))
	}

	// Add volume mounts
	for _, volume := range config.Volumes {
		mountStr := fmt.Sprintf("%s:%s", volume.Source, volume.Target)
		if volume.ReadOnly {
			mountStr += ":ro"
		}
		args = append(args, "-v", mountStr)
	}

	// Add image
	args = append(args, config.Image)

	// Add command if specified
	if len(config.Command) > 0 {
		args = append(args, config.Command...)
	}

	cm.logger.Debug("starting container", zap.Strings("args", args))

	// Execute docker run command
	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to start container: %v (output: %s)", err, string(output))
	}

	containerID := strings.TrimSpace(string(output))
	if containerID == "" {
		return nil, fmt.Errorf("docker run returned empty container ID")
	}

	cm.logger.Debug("container started", zap.String("container_id", containerID))

	// Get container IP and port
	container, err := cm.getContainerInfo(ctx, containerID, config.Port)
	if err != nil {
		// Clean up the container if we can't get its info
		cm.logger.Warn("Failed to get container info, attempting to stop container", zap.String("container_id", containerID), zap.Error(err))
		if stopErr := cm.stopContainerByID(ctx, containerID); stopErr != nil {
			cm.logger.Error("Failed to stop container after failing to get its info", zap.String("container_id", containerID), zap.Error(stopErr))
			// Return an error that includes both the original error and the stop error
			return nil, fmt.Errorf("failed to get container info for %s: %w; additionally, failed to stop container: %v", containerID, err, stopErr)
		}
		cm.logger.Info("Successfully stopped container after failing to get its info", zap.String("container_id", containerID))
		// Return the original error, noting that the container was stopped
		return nil, fmt.Errorf("failed to get container info for %s: %w (container has been stopped)", containerID, err)
	}

	// Store container reference
	cm.mutex.Lock()
	cm.containers[containerID] = container
	cm.mutex.Unlock()

	return container, nil
}

// getContainerInfo retrieves the IP address and port mapping for a container
func (cm *ContainerManager) getContainerInfo(ctx context.Context, containerID string, internalPort int) (*Container, error) {
	// With host networking, containers use localhost
	// We just need to verify the container exists
	cmd := exec.CommandContext(ctx, "docker", "inspect", containerID)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %v", err)
	}

	// Parse JSON output to verify container exists
	var inspectData []map[string]interface{}
	if err := json.Unmarshal(output, &inspectData); err != nil {
		return nil, fmt.Errorf("failed to parse container inspect output: %v", err)
	}

	if len(inspectData) == 0 {
		return nil, fmt.Errorf("no container data returned")
	}

	// With host networking, use localhost and the configured port
	return &Container{
		ID:   containerID,
		IP:   "127.0.0.1",
		Port: internalPort,
	}, nil
}

// WaitForReady waits for the container to be ready to accept connections
func (cm *ContainerManager) WaitForReady(ctx context.Context, container *Container, timeout time.Duration, port int) error {
	deadline := time.Now().Add(timeout)
	
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// With host networking, connect to localhost on the configured port
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
		if err == nil {
			conn.Close()
			cm.logger.Info("container is ready", 
				zap.String("container_id", container.ID),
				zap.String("ip", "127.0.0.1"),
				zap.Int("port", port))
			return nil
		}

		// Wait a bit before retrying
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("container did not become ready within timeout")
}

// StopContainer stops and removes a container
func (cm *ContainerManager) StopContainer(ctx context.Context, containerID string) error {
	cm.mutex.Lock()
	delete(cm.containers, containerID)
	cm.mutex.Unlock()

	return cm.stopContainerByID(ctx, containerID)
}

// stopContainerByID stops a container by its ID
func (cm *ContainerManager) stopContainerByID(ctx context.Context, containerID string) error {
	cm.logger.Debug("stopping container", zap.String("container_id", containerID))

	cmd := exec.CommandContext(ctx, "docker", "stop", containerID)
	if err := cmd.Run(); err != nil {
		stopErr := err
		cm.logger.Warn("failed to stop container", zap.String("container_id", containerID), zap.Error(err))
		// Try to force remove it
		cmd = exec.CommandContext(ctx, "docker", "rm", "-f", containerID)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to force remove container (stop error: %v): %v", stopErr, err)
		}
	}

	return nil
}

// Cleanup stops all managed containers
func (cm *ContainerManager) Cleanup() error {
	cm.mutex.Lock()
	containerIDs := make([]string, 0, len(cm.containers))
	for id := range cm.containers {
		containerIDs = append(containerIDs, id)
	}
	cm.containers = make(map[string]*Container)
	cm.mutex.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var lastErr error
	for _, id := range containerIDs {
		if err := cm.stopContainerByID(ctx, id); err != nil {
			cm.logger.Error("failed to stop container during cleanup", zap.String("container_id", id), zap.Error(err))
			lastErr = err
		}
	}

	return lastErr
}

// HealthCheck performs a health check on a container
func (cm *ContainerManager) HealthCheck(ctx context.Context, container *Container) error {
	url := fmt.Sprintf("http://%s:%d/health", container.IP, container.Port)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := cm.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed with status %d", resp.StatusCode)
	}

	return nil
}
