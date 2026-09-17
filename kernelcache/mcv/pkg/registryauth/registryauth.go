/*
Copyright 2026 The KServe Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package registryauth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

const (
	caFileEnv           = "MCV_REGISTRY_CA_FILE"        // #nosec G101 -- this is an environment variable name, not a credential
	tokenFileEnv        = "MCV_REGISTRY_TOKEN_FILE"     // #nosec G101 -- this is an environment variable name, not a credential
	tokenRegistryEnv    = "MCV_REGISTRY_TOKEN_REGISTRY" // #nosec G101 -- this is an environment variable name, not a credential
	accessFileEnv       = "MCV_REGISTRY_ACCESS_FILE"
	legacyAccessFileEnv = "MCV_REGISTRY_CREDENTIAL_FILE" // #nosec G101 -- compatibility with older injected Pods
)

type publishingCredential struct {
	Registry  string    `json:"registry"`
	Username  string    `json:"username"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// RemoteOptions returns registry options for pull and push operations.
func RemoteOptions(ctx context.Context, targetRegistry string) ([]remote.Option, error) {
	options := []remote.Option{remote.WithContext(ctx)}

	if caFile := strings.TrimSpace(os.Getenv(caFileEnv)); caFile != "" {
		transport, err := registryTransport(caFile)
		if err != nil {
			return nil, err
		}
		options = append(options, remote.WithTransport(transport))
	}

	accessFile := strings.TrimSpace(os.Getenv(accessFileEnv))
	if accessFile == "" {
		accessFile = strings.TrimSpace(os.Getenv(legacyAccessFileEnv))
	}
	if accessFile != "" {
		if err := ValidateTokenScope(targetRegistry); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(filepath.Clean(accessFile))
		if err != nil {
			return nil, fmt.Errorf("read registry access: %w", err)
		}
		var access publishingCredential
		if err := json.Unmarshal(data, &access); err != nil {
			return nil, fmt.Errorf("invalid registry access JSON: %w", err)
		}
		if access.Token == "" || access.Username == "" || !access.ExpiresAt.After(time.Now()) {
			return nil, errors.New("registry access is missing or expired")
		}
		if !strings.EqualFold(access.Registry, targetRegistry) {
			return nil, errors.New("registry access target mismatch")
		}
		options = append(options, remote.WithAuth(authn.FromConfig(authn.AuthConfig{Username: access.Username, Password: access.Token})))
	} else if os.Getenv("MCV_REGISTRY_AUTH_REQUIRED") == "true" {
		return nil, errors.New("registry access file is required")
	} else if tokenFile := strings.TrimSpace(os.Getenv(tokenFileEnv)); tokenFile != "" {
		if err := ValidateTokenScope(targetRegistry); err != nil {
			return nil, err
		}
		token, err := os.ReadFile(filepath.Clean(tokenFile))
		if err != nil {
			return nil, fmt.Errorf("read registry token: %w", err)
		}
		if tokenValue := strings.TrimSpace(string(token)); tokenValue != "" {
			options = append(options, remote.WithAuth(authn.FromConfig(authn.AuthConfig{
				Username: "serviceaccount",
				Password: tokenValue,
			})))
		} else {
			return nil, fmt.Errorf("registry token file is empty: %s", tokenFile)
		}
	} else {
		options = append(options, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	}

	return options, nil
}

// ValidateTokenScope prevents a token from being sent to an unintended registry.
func ValidateTokenScope(targetRegistry string) error {
	configuredRegistry := strings.TrimSpace(os.Getenv(tokenRegistryEnv))
	if configuredRegistry == "" {
		return errors.New("MCV_REGISTRY_TOKEN_REGISTRY is required when registry credentials are configured")
	}

	allowed, err := name.NewRegistry(configuredRegistry, name.StrictValidation)
	if err != nil {
		return fmt.Errorf("invalid MCV_REGISTRY_TOKEN_REGISTRY: %w", err)
	}
	target, err := name.NewRegistry(targetRegistry, name.StrictValidation)
	if err != nil {
		return fmt.Errorf("invalid target registry: %w", err)
	}
	if !strings.EqualFold(allowed.RegistryStr(), target.RegistryStr()) {
		return fmt.Errorf("registry token is scoped to %q, not target registry %q", allowed.RegistryStr(), target.RegistryStr())
	}
	return nil
}

func registryTransport(caFile string) (http.RoundTripper, error) {
	caPEM, err := os.ReadFile(filepath.Clean(caFile))
	if err != nil {
		return nil, fmt.Errorf("read registry CA: %w", err)
	}

	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("load system cert pool: %w", err)
	}
	if roots == nil {
		roots = x509.NewCertPool()
	}
	if ok := roots.AppendCertsFromPEM(caPEM); !ok {
		return nil, fmt.Errorf("registry CA file contains no valid certificates: %s", caFile)
	}

	base, ok := remote.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("unsupported default HTTP transport")
	}
	transport := base.Clone()
	tlsConfig := transport.TLSClientConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	} else {
		tlsConfig = tlsConfig.Clone()
	}
	tlsConfig.MinVersion = tls.VersionTLS12
	tlsConfig.RootCAs = roots
	transport.TLSClientConfig = tlsConfig
	return transport, nil
}
