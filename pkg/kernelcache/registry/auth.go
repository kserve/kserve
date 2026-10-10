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

package registry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

const serviceAccountMountPath = "/var/run/secrets/kubernetes.io/serviceaccount"

var projectedCredentialPath = path.Join(serviceAccountMountPath, "token")

var readServiceAccountToken = os.ReadFile

// RegistryAccess contains the registry connection settings used by the
// localmodel controller for artifact signing and verification.
type RegistryAccess struct {
	Transport     http.RoundTripper
	Authenticator authn.Authenticator
	Insecure      bool
}

// NewControllerRegistryAccess builds registry connection settings for an image
// in a workload namespace. It loads the configured CA bundle and obtains the
// controller ServiceAccount credential when registry authentication is enabled.
func NewControllerRegistryAccess(ctx context.Context, reader client.Reader, namespace string, config v1beta1.KernelCacheRegistryConfig, imageRef string) (RegistryAccess, error) {
	transport, err := NewTransport(ctx, reader, namespace, config)
	if err != nil {
		return RegistryAccess{}, err
	}
	authenticator, err := NewControllerServiceAccountAuthenticator(config, imageRef)
	if err != nil {
		return RegistryAccess{}, err
	}
	return RegistryAccess{Transport: transport, Authenticator: authenticator, Insecure: config.Insecure}, nil
}

// NewControllerServiceAccountAuthenticator returns the controller Pod ServiceAccount token
// for the configured registry. The token is read for each reconcile so a
// rotated projected token is picked up without restarting the controller.
func NewControllerServiceAccountAuthenticator(config v1beta1.KernelCacheRegistryConfig, imageRef string) (authn.Authenticator, error) {
	if config.Auth.Type == "" || config.Auth.Type == v1beta1.KernelCacheRegistryAuthTypeNone {
		return nil, nil
	}
	if config.Auth.Type != v1beta1.KernelCacheRegistryAuthTypeServiceAccountToken {
		return nil, fmt.Errorf("unsupported registry authentication provider %q", config.Auth.Type)
	}

	options := []name.Option{}
	if config.Insecure {
		options = append(options, name.Insecure)
	}
	ref, err := name.ParseReference(imageRef, options...)
	if err != nil {
		return nil, fmt.Errorf("parse registry image reference %q: %w", imageRef, err)
	}
	configured, err := name.NewRegistry(config.Endpoint, name.StrictValidation)
	if err != nil {
		return nil, fmt.Errorf("parse configured registry endpoint %q: %w", config.Endpoint, err)
	}
	if !strings.EqualFold(ref.Context().RegistryStr(), configured.RegistryStr()) {
		return nil, fmt.Errorf("registry image %q is outside configured endpoint %q", ref.Context().RegistryStr(), configured.RegistryStr())
	}

	token, err := readServiceAccountToken(projectedCredentialPath)
	if err != nil {
		return nil, fmt.Errorf("read controller ServiceAccount token: %w", err)
	}
	if token = []byte(strings.TrimSpace(string(token))); len(token) == 0 {
		return nil, errors.New("controller ServiceAccount token is empty")
	}
	return authn.FromConfig(authn.AuthConfig{Username: "unused", Password: string(token)}), nil
}
