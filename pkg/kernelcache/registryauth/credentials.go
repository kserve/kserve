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
	"encoding/json"
	"errors"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	controllerclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

// Credentials is called only after the registry reconciler verifies the Pod owner chain.
// No callback endpoint is registered by this package.
type Credentials struct {
	Client kubernetes.Interface
}

// IssueForCapture publishes a Secret-bound token using the registry identity
// scoped to one KernelCacheCapture.
func (c *Credentials) IssueForCapture(ctx context.Context, pod *corev1.Pod, captureName string, cfg v1beta1.KernelCacheRegistryConfig) (*corev1.Secret, error) {
	if captureName == "" {
		return nil, errors.New("registry credential requires a capture name")
	}
	provider, err := NewProvider(c.Client, cfg)
	if err != nil {
		return nil, err
	}
	if cfg.Auth.Type == "" || cfg.Auth.Type == v1beta1.KernelCacheRegistryAuthTypeNone {
		return nil, nil
	}
	live, err := c.Client.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if live.UID != pod.UID || live.UID == "" || live.DeletionTimestamp != nil {
		return nil, apierrors.NewResourceExpired("capture Pod is no longer active")
	}
	name := accessSecretName(live)
	if !strings.HasPrefix(name, "mcv-registry-") {
		return nil, errors.New("capture Pod has no registry access Secret reference")
	}
	secrets := c.Client.CoreV1().Secrets(live.Namespace)
	secret, err := secrets.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		owner := metav1.NewControllerRef(live, corev1.SchemeGroupVersion.WithKind("Pod"))
		// Access cleanup must not block runtime Pod deletion.
		blockOwnerDeletion := false
		owner.BlockOwnerDeletion = &blockOwnerDeletion
		secret, err = secrets.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: name, Namespace: live.Namespace,
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
		if !ownedByPod(current, live) || current.UID != secret.UID || current.DeletionTimestamp != nil {
			return errors.New("registry access Secret does not belong to this capture Pod")
		}
		var existing RegistryCredential
		if json.Unmarshal(current.Data[AccessKey], &existing) == nil &&
			existing.Token != "" && existing.Registry == cfg.Endpoint && existing.ExpiresAt.After(time.Now().Add(time.Minute)) {
			updated = current
			return nil
		}
		credential, err := provider.GetCredential(ctx, CredentialRequest{
			Registry:           cfg.Endpoint,
			ServiceAccountName: PusherServiceAccountName(captureName),
			Secret:             corev1.ObjectReference{APIVersion: "v1", Kind: "Secret", Namespace: current.Namespace, Name: current.Name, UID: current.UID},
		})
		if err != nil {
			return err
		}
		now := time.Now()
		// Use the actual expiration, not the requested lifetime.
		if !credential.ExpiresAt.After(now) || credential.ExpiresAt.After(now.Add(time.Hour+30*time.Second)) {
			return errors.New("TokenRequest returned an unacceptable expiration")
		}
		data, err := json.Marshal(credential)
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

// Revoke deletes the bound object; clearing data alone does not revoke a token.
func Revoke(ctx context.Context, c controllerclient.Client, pod *corev1.Pod) error {
	name := accessSecretName(pod)
	if name == "" {
		return nil
	}
	secret := &corev1.Secret{}
	err := c.Get(ctx, controllerclient.ObjectKey{Namespace: pod.Namespace, Name: name}, secret)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !ownedByPod(secret, pod) {
		return errors.New("registry access Secret does not belong to this capture Pod")
	}
	uid := secret.UID
	resourceVersion := secret.ResourceVersion
	return controllerclient.IgnoreNotFound(c.Delete(ctx, secret, controllerclient.Preconditions{
		UID:             &uid,
		ResourceVersion: &resourceVersion,
	}))
}

func accessSecretName(pod *corev1.Pod) string {
	return pod.Annotations[AccessSecretAnnotation]
}

func ownedByPod(secret *corev1.Secret, pod *corev1.Pod) bool {
	owner := metav1.GetControllerOf(secret)
	return pod.UID != "" && secret.Labels[ManagedLabel] == "true" &&
		owner != nil && owner.APIVersion == "v1" && owner.Kind == "Pod" && owner.Name == pod.Name && owner.UID == pod.UID
}
