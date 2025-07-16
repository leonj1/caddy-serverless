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
	"fmt"
	"regexp" // Added for environment variable name validation
	"strconv"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func init() {
	httpcaddyfile.RegisterHandlerDirective("serverless", parseCaddyfile)
}

// UnmarshalCaddyfile sets up the handler from Caddyfile tokens. Syntax:
//
//	serverless {
//	    function {
//	        methods GET POST
//	        path /api/.*
//	        image nginx:latest
//	        command /bin/sh -c "echo hello"
//	        env KEY=value
//	        volume /host/path:/container/path
//	        volume /host/path:/container/path:ro
//	        timeout 30s
//	        port 8080
//	    }
//	}
func (h *ServerlessHandler) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	d.Next() // consume directive name

	for d.NextBlock(0) {
		switch d.Val() {
		case "function":
			function := FunctionConfig{
				Environment: make(map[string]string),
			}

			for d.NextBlock(1) {
				switch d.Val() {
				case "methods":
					args := d.RemainingArgs()
					if len(args) == 0 {
						return d.ArgErr()
					}
					function.Methods = args

				case "path":
					if !d.NextArg() {
						return d.ArgErr()
					}
					function.Path = d.Val()

				case "image":
					if !d.NextArg() {
						return d.ArgErr()
					}
					function.Image = d.Val()

				case "command":
					args := d.RemainingArgs()
					if len(args) == 0 {
						return d.ArgErr()
					}
					function.Command = args

				case "env":
					if !d.NextArg() {
						return d.ArgErr()
					}
					envVar := d.Val()
					parts := strings.SplitN(envVar, "=", 2)
					if len(parts) != 2 {
						// This handles cases like "KEY" without "=", ensuring "KEY=value" structure.
						// For "KEY=", parts will be ["KEY", ""], so len(parts) == 2, which is valid.
						return d.Errf("invalid environment variable format: %s (expected KEY=value)", envVar)
					}

					key := parts[0]
					value := parts[1]

					// Validate environment variable name
					if key == "" {
						return d.Errf("environment variable name cannot be empty")
					}
					// Regex for valid env var names: must start with a letter or underscore,
					// and can only contain letters, numbers, or underscores.
					// This aligns with common practices (e.g., POSIX-like, but allowing lowercase).
					isValidName, _ := regexp.MatchString(`^[a-zA-Z_][a-zA-Z0-9_]*$`, key) // Error from MatchString is ignored as the regex is constant.
					if !isValidName {
						return d.Errf("invalid environment variable name: '%s'. Name must start with a letter or underscore, and can only contain letters, numbers, or underscores.", key)
					}

					// Store the environment variable. Empty values (e.g., "KEY=") are allowed and will be stored as empty strings.
					function.Environment[key] = value

				case "volume":
					if !d.NextArg() {
						return d.ArgErr()
					}
					volumeSpec := d.Val()
					volume, err := parseVolumeSpec(volumeSpec)
					if err != nil {
						return d.Errf("invalid volume specification: %v", err)
					}
					function.Volumes = append(function.Volumes, volume)

				case "timeout":
					if !d.NextArg() {
						return d.ArgErr()
					}
					timeoutStr := d.Val()
					timeout, err := time.ParseDuration(timeoutStr)
					if err != nil {
						return d.Errf("invalid timeout duration: %v", err)
					}
					function.Timeout = caddy.Duration(timeout)

				case "port":
					if !d.NextArg() {
						return d.ArgErr()
					}
					portStr := d.Val()
					port, err := strconv.Atoi(portStr)
					if err != nil {
						return d.Errf("invalid port number: %v", err)
					}
					if port < 1 || port > 65535 {
						return d.Errf("port number must be between 1 and 65535")
					}
					function.Port = port

				default:
					return d.Errf("unrecognized subdirective '%s'", d.Val())
				}
			}

			// After the function configuration block
			if function.Image == "" {
				return d.Errf("image is required for serverless function")
			}
			if function.Path == "" {
				return d.Errf("path is required for serverless function")
			}
			h.Functions = append(h.Functions, function)

		default:
			return d.Errf("unrecognized subdirective '%s'", d.Val())
		}
	}

	return nil
}

// parseVolumeSpec parses a volume specification in the format:
// /host/path:/container/path[:ro]
func parseVolumeSpec(spec string) (VolumeMount, error) {
	parts := strings.Split(spec, ":")
	const (
		minVolumeSpecParts = 2
		maxVolumeSpecParts = 3
	)
	if len(parts) < minVolumeSpecParts || len(parts) > maxVolumeSpecParts {
		return VolumeMount{}, fmt.Errorf("invalid volume format (expected /host/path:/container/path[:ro])")
	}

	volume := VolumeMount{
		Source: parts[0],
		Target: parts[1],
	}

	if len(parts) == 3 {
		if parts[2] == "ro" {
			volume.ReadOnly = true
		} else {
			return VolumeMount{}, fmt.Errorf("invalid volume option '%s' (only 'ro' is supported)", parts[2])
		}
	}

	return volume, nil
}

// parseCaddyfile parses the serverless directive from a Caddyfile
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var handler ServerlessHandler
	err := handler.UnmarshalCaddyfile(h.Dispenser)
	return handler, err
}

// Interface guard
var _ caddyfile.Unmarshaler = (*ServerlessHandler)(nil)
