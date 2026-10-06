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
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestKubernetesSecretSource(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "kernel-cache-ca", Namespace: "kserve"},
		Data:       map[string][]byte{"ca.crt": []byte("test")},
	}
	source := NewKubernetesSecretSource(fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build())

	data, err := source.GetSecret(t.Context(), "kserve/kernel-cache-ca")
	require.NoError(t, err)
	require.Equal(t, []byte("test"), data["ca.crt"])

	_, err = source.GetSecret(t.Context(), "kernel-cache-ca")
	require.Error(t, err)
}
