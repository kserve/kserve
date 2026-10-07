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

package security

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type kubernetesSecretSource struct {
	reader client.Reader
}

var _ SecretSource = (*kubernetesSecretSource)(nil)

// NewKubernetesSecretSource loads security material through a Kubernetes client.
func NewKubernetesSecretSource(reader client.Reader) SecretSource {
	return &kubernetesSecretSource{reader: reader}
}

func (s *kubernetesSecretSource) GetSecret(ctx context.Context, ref string) (map[string][]byte, error) {
	namespace, name, err := splitReference(ref)
	if err != nil {
		return nil, err
	}
	secret := &corev1.Secret{}
	if err := s.reader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, secret); err != nil {
		return nil, err
	}
	return secret.Data, nil
}

func (s *kubernetesSecretSource) GetConfigMap(ctx context.Context, ref string) (map[string]string, error) {
	namespace, name, err := splitReference(ref)
	if err != nil {
		return nil, err
	}
	configMap := &corev1.ConfigMap{}
	if err := s.reader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, configMap); err != nil {
		return nil, err
	}
	return configMap.Data, nil
}

func splitReference(ref string) (string, string, error) {
	namespace, name, err := cache.SplitMetaNamespaceKey(ref)
	if err != nil || namespace == "" || name == "" {
		return "", "", fmt.Errorf("reference %q must use namespace/name", ref)
	}
	return namespace, name, nil
}
