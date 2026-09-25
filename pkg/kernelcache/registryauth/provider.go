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
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

const (
	TokenRequesterRole     = "kserve-kernelcache-token-requester" // #nosec G101 -- this is an RBAC role name, not a credential
	ManagedLabel           = "internal.serving.kserve.io/kernelcache-registry"
	AccessSecretAnnotation = "internal.serving.kserve.io/kernelcache-access-secret"
	AccessKey              = "access.json"
)

const (
	pusherServiceAccountPrefix = "kernel-cache-pusher-"
	pusherRoleBindingPrefix    = "kernel-cache-pusher-"
)

// PusherServiceAccountName returns the registry identity scoped to one KCC.
func PusherServiceAccountName(captureName string) string {
	return scopedName(pusherServiceAccountPrefix, captureName)
}

// PusherRoleBindingName returns the registry binding scoped to one KCC.
func PusherRoleBindingName(captureName string) string {
	return scopedName(pusherRoleBindingPrefix, captureName)
}

func scopedName(prefix, captureName string) string {
	digest := sha256.Sum256([]byte(captureName))
	suffix := fmt.Sprintf("-%x", digest[:4])
	maxBase := 63 - len(prefix) - len(suffix)
	if maxBase < 1 {
		return prefix + strings.TrimPrefix(suffix, "-")
	}
	base := captureName
	if len(base) > maxBase {
		base = strings.TrimRight(base[:maxBase], ".-")
	}
	return prefix + base + suffix
}

type CredentialRequest struct {
	Secret             corev1.ObjectReference
	Registry           string
	ServiceAccountName string
}

// RegistryCredential must never be included in logs or CR status.
type RegistryCredential struct {
	Registry  string      `json:"registry"`
	Username  string      `json:"username"`
	Token     string      `json:"token"`
	ExpiresAt metav1.Time `json:"expiresAt"`
}

type RegistryAuthProvider interface {
	GetCredential(context.Context, CredentialRequest) (*RegistryCredential, error)
}

type NoAuthProvider struct{}

func (NoAuthProvider) GetCredential(context.Context, CredentialRequest) (*RegistryCredential, error) {
	return nil, nil
}

// ServiceAccountTokenProvider issues short-lived registry credentials through
// the Kubernetes TokenRequest API.
type ServiceAccountTokenProvider struct {
	Client     kubernetes.Interface
	TTLSeconds int64
}

func NewProvider(c kubernetes.Interface, cfg v1beta1.KernelCacheRegistryConfig) (RegistryAuthProvider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	switch cfg.Auth.Type {
	case "", v1beta1.KernelCacheRegistryAuthTypeNone:
		return NoAuthProvider{}, nil
	case v1beta1.KernelCacheRegistryAuthTypeServiceAccountToken:
		ttl := cfg.Auth.TokenTTLSeconds
		if ttl == 0 {
			ttl = v1beta1.DefaultKernelCacheRegistryTokenTTLSeconds
		}
		return &ServiceAccountTokenProvider{Client: c, TTLSeconds: ttl}, nil
	default:
		return nil, fmt.Errorf("unsupported registry authentication provider %q", cfg.Auth.Type)
	}
}

func (p *ServiceAccountTokenProvider) GetCredential(ctx context.Context, req CredentialRequest) (*RegistryCredential, error) {
	ref := req.Secret
	if ref.Kind != "Secret" || ref.APIVersion != "v1" || ref.Namespace == "" || ref.Name == "" || ref.UID == "" || req.Registry == "" || req.ServiceAccountName == "" {
		return nil, errors.New("registry access requires an existing Secret name, namespace and UID")
	}
	result, err := p.Client.CoreV1().ServiceAccounts(ref.Namespace).CreateToken(ctx, req.ServiceAccountName, &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &p.TTLSeconds,
			BoundObjectRef:    &authenticationv1.BoundObjectReference{Kind: "Secret", APIVersion: "v1", Name: ref.Name, UID: ref.UID},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("request registry access token: %w", err)
	}
	if result.Status.Token == "" {
		return nil, errors.New("TokenRequest returned empty registry access")
	}
	return &RegistryCredential{Registry: req.Registry, Username: "unused", Token: result.Status.Token, ExpiresAt: result.Status.ExpirationTimestamp}, nil
}
