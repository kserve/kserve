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

package fixture

import (
	"context"
	"fmt"

	"github.com/kserve/kserve/pkg/kernelcache/security"
)

// FakeSecretSource is an in-memory security.SecretSource for tests. Keys use the
// same "namespace/name" reference form the real source expects.
type FakeSecretSource struct {
	Secrets    map[string]map[string][]byte
	ConfigMaps map[string]map[string]string
}

var _ security.SecretSource = (*FakeSecretSource)(nil)

// GetSecret returns a copy of the stored Secret data, or an error if the ref is
// absent. The copy (map and byte slices) prevents a caller from mutating the
// source's stored data.
func (m *FakeSecretSource) GetSecret(_ context.Context, ref string) (map[string][]byte, error) {
	data, ok := m.Secrets[ref]
	if !ok {
		return nil, fmt.Errorf("secret %q not found", ref)
	}
	out := make(map[string][]byte, len(data))
	for k, v := range data {
		out[k] = append([]byte(nil), v...)
	}
	return out, nil
}

// GetConfigMap returns a copy of the stored ConfigMap data, or an error if the
// ref is absent.
func (m *FakeSecretSource) GetConfigMap(_ context.Context, ref string) (map[string]string, error) {
	data, ok := m.ConfigMaps[ref]
	if !ok {
		return nil, fmt.Errorf("configmap %q not found", ref)
	}
	out := make(map[string]string, len(data))
	for k, v := range data {
		out[k] = v
	}
	return out, nil
}
