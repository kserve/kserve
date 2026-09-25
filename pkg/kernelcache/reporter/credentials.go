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

package reporter

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

const (
	AccessSecretAnnotation       = "internal.serving.kserve.io/kernelcache-reporter-secret"
	AccessKey                    = "access.json"
	ManagedLabel                 = "internal.serving.kserve.io/kernelcache-reporter"
	tokenRefreshBuffer           = time.Minute
	defaultTokenTTLSeconds int64 = 600
	reporterResourcePrefix       = "kernel-cache-reporter-"
)

// ServiceAccountName returns the reporter identity name scoped to one KCC.
func ServiceAccountName(captureName string) string {
	return scopedName(captureName)
}

// RoleName returns the Role name scoped to one KCC.
func RoleName(captureName string) string { return scopedName(captureName) }

// RoleBindingName returns the RoleBinding name scoped to one KCC.
func RoleBindingName(captureName string) string { return scopedName(captureName) }

// SecretName returns the deterministic reporter Secret name scoped to one KCC.
func SecretName(captureName string) string { return scopedName(captureName) }

func scopedName(captureName string) string {
	digest := sha256.Sum256([]byte(captureName))
	suffix := fmt.Sprintf("-%x", digest[:4])
	maxBase := 63 - len(reporterResourcePrefix) - len(suffix)
	if maxBase < 1 {
		return reporterResourcePrefix + strings.TrimPrefix(suffix, "-")
	}
	base := captureName
	if len(base) > maxBase {
		base = strings.TrimRight(base[:maxBase], "-.")
	}
	return reporterResourcePrefix + base + suffix
}

// IsReporterUsername identifies authenticated MCV reporter ServiceAccounts.
// The namespace is checked separately by admission code because the username
// format contains it as a path component.
func IsReporterUsername(username string) bool {
	const prefix = "system:serviceaccount:"
	if !strings.HasPrefix(username, prefix) {
		return false
	}
	parts := strings.Split(username[len(prefix):], ":")
	return len(parts) == 2 && strings.HasPrefix(parts[1], reporterResourcePrefix)
}

// IsReporterForCapture verifies that a reporter identity is scoped to the KCC
// being updated.
func IsReporterForCapture(username, namespace, captureName string) bool {
	const prefix = "system:serviceaccount:"
	if !strings.HasPrefix(username, prefix) {
		return false
	}
	parts := strings.Split(username[len(prefix):], ":")
	if len(parts) != 2 || parts[0] != namespace {
		return false
	}
	return parts[1] == ServiceAccountName(captureName)
}

// Credential is a short-lived token for updating one KernelCacheCapture status.
type Credential struct {
	Token     string      `json:"token"`
	ExpiresAt metav1.Time `json:"expiresAt"`
}

// Credentials creates a reporter credential scoped to one KernelCacheCapture.
type Credentials struct {
	Client kubernetes.Interface
}

// IssueForCapture creates or refreshes a reporter credential scoped to one
// KCC. The Secret may be shared by all candidate Pods for that KCC.
func (c *Credentials) IssueForCapture(ctx context.Context, pod *corev1.Pod, captureName string, owner *metav1.OwnerReference) (*corev1.Secret, error) {
	if captureName == "" || owner == nil {
		return nil, errors.New("capture reporter credential requires a capture owner")
	}
	return c.issue(ctx, pod, ServiceAccountName(captureName), SecretName(captureName), owner)
}

func (c *Credentials) issue(ctx context.Context, pod *corev1.Pod, serviceAccountName, requestedSecretName string, owner *metav1.OwnerReference) (*corev1.Secret, error) {
	live, err := c.Client.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if live.UID != pod.UID || live.UID == "" || live.DeletionTimestamp != nil {
		return nil, apierrors.NewResourceExpired("capture Pod is no longer active")
	}
	name := live.Annotations[AccessSecretAnnotation]
	if name != requestedSecretName {
		return nil, errors.New("capture Pod has no reporter access Secret reference")
	}

	secrets := c.Client.CoreV1().Secrets(live.Namespace)
	secret, err := secrets.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		secret, err = secrets.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				Namespace:       live.Namespace,
				Labels:          map[string]string{ManagedLabel: "true"},
				OwnerReferences: []metav1.OwnerReference{*owner},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{AccessKey: []byte("{}")},
		}, metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(err) {
			secret, err = secrets.Get(ctx, name, metav1.GetOptions{})
		}
	}
	if err != nil {
		return nil, err
	}

	var updated *corev1.Secret
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, err := secrets.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !ownedByCapture(current, owner) || current.UID != secret.UID || current.DeletionTimestamp != nil {
			return errors.New("reporter access Secret does not belong to this capture")
		}
		var existing Credential
		if json.Unmarshal(current.Data[AccessKey], &existing) == nil && existing.Token != "" && existing.ExpiresAt.After(time.Now().Add(tokenRefreshBuffer)) {
			updated = current
			return nil
		}
		ttl := defaultTokenTTLSeconds
		result, err := c.Client.CoreV1().ServiceAccounts(current.Namespace).CreateToken(ctx, serviceAccountName, &authenticationv1.TokenRequest{
			Spec: authenticationv1.TokenRequestSpec{
				ExpirationSeconds: &ttl,
				BoundObjectRef:    &authenticationv1.BoundObjectReference{Kind: "Secret", APIVersion: "v1", Name: current.Name, UID: current.UID},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("request reporter token: %w", err)
		}
		if result.Status.Token == "" || !result.Status.ExpirationTimestamp.After(time.Now()) {
			return errors.New("TokenRequest returned an unacceptable reporter access")
		}
		data, err := json.Marshal(Credential{Token: result.Status.Token, ExpiresAt: result.Status.ExpirationTimestamp})
		if err != nil {
			return err
		}
		if current.Data == nil {
			current.Data = map[string][]byte{}
		}
		current.Data[AccessKey] = data
		updated, err = secrets.Update(ctx, current, metav1.UpdateOptions{})
		return err
	})
	return updated, err
}

func ownedByCapture(secret *corev1.Secret, captureOwner *metav1.OwnerReference) bool {
	owner := metav1.GetControllerOf(secret)
	if secret.Labels[ManagedLabel] != "true" || owner == nil {
		return false
	}
	return owner.APIVersion == captureOwner.APIVersion && owner.Kind == captureOwner.Kind &&
		owner.Name == captureOwner.Name && owner.UID == captureOwner.UID
}
