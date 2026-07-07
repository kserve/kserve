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

package llmisvc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials/s3"
)

// fakeCSCClient builds a controller-runtime fake client seeded with the given
// ClusterStorageContainers.
func fakeCSCClient(t *testing.T, cscs ...*v1alpha1.ClusterStorageContainer) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	builder := fakeclient.NewClientBuilder().WithScheme(scheme)
	for _, csc := range cscs {
		builder = builder.WithObjects(csc)
	}
	return builder.Build()
}

func csc(name string, disabled *bool, workloadType v1alpha1.WorkloadType, prefixes ...string) *v1alpha1.ClusterStorageContainer {
	supported := make([]v1alpha1.SupportedUriFormat, 0, len(prefixes))
	for _, p := range prefixes {
		supported = append(supported, v1alpha1.SupportedUriFormat{Prefix: p})
	}
	return &v1alpha1.ClusterStorageContainer{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Disabled:   disabled,
		Spec: v1alpha1.StorageContainerSpec{
			Container:           corev1.Container{Name: "storage-initializer"},
			SupportedUriFormats: supported,
			WorkloadType:        workloadType,
		},
	}
}

func TestResolveStorageContainerSpec(t *testing.T) {
	tests := []struct {
		name         string
		cscs         []*v1alpha1.ClusterStorageContainer
		modelURI     string
		explicitName string
		wantNil      bool
		wantErr      string
	}{
		{
			name:     "auto-match by URI prefix",
			cscs:     []*v1alpha1.ClusterStorageContainer{csc("default", nil, v1alpha1.InitContainer, "s3://")},
			modelURI: "s3://bucket/model",
		},
		{
			name:     "no match returns nil, nil",
			cscs:     []*v1alpha1.ClusterStorageContainer{csc("default", nil, v1alpha1.InitContainer, "s3://")},
			modelURI: "hf://user/model",
			wantNil:  true,
		},
		{
			name:     "disabled CSC is skipped on auto-match",
			cscs:     []*v1alpha1.ClusterStorageContainer{csc("default", ptr.To(true), v1alpha1.InitContainer, "s3://")},
			modelURI: "s3://bucket/model",
			wantNil:  true,
		},
		{
			name:     "wrong workload type is skipped on auto-match",
			cscs:     []*v1alpha1.ClusterStorageContainer{csc("default", nil, v1alpha1.LocalModelDownloadJob, "s3://")},
			modelURI: "s3://bucket/model",
			wantNil:  true,
		},
		{
			name:         "explicit by-name selection succeeds",
			cscs:         []*v1alpha1.ClusterStorageContainer{csc("chosen", nil, v1alpha1.InitContainer, "s3://")},
			modelURI:     "s3://bucket/model",
			explicitName: "chosen",
		},
		{
			name:         "explicit by-name for missing CSC returns error",
			cscs:         nil,
			modelURI:     "s3://bucket/model",
			explicitName: "chosen",
			wantErr:      `ClusterStorageContainer "chosen" not found`,
		},
		{
			name:         "explicit by-name for disabled CSC returns error",
			cscs:         []*v1alpha1.ClusterStorageContainer{csc("chosen", ptr.To(true), v1alpha1.InitContainer, "s3://")},
			modelURI:     "s3://bucket/model",
			explicitName: "chosen",
			wantErr:      `ClusterStorageContainer "chosen" is disabled`,
		},
		{
			name:         "explicit by-name with wrong workload type returns error",
			cscs:         []*v1alpha1.ClusterStorageContainer{csc("chosen", nil, v1alpha1.LocalModelDownloadJob, "s3://")},
			modelURI:     "s3://bucket/model",
			explicitName: "chosen",
			wantErr:      "workloadType",
		},
		{
			name:         "explicit by-name for CSC that does not support the URI returns error",
			cscs:         []*v1alpha1.ClusterStorageContainer{csc("chosen", nil, v1alpha1.InitContainer, "hf://")},
			modelURI:     "s3://bucket/model",
			explicitName: "chosen",
			wantErr:      `does not support storageUri "s3://bucket/model"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := fakeCSCClient(t, tc.cscs...)
			spec, err := resolveStorageContainerSpec(context.Background(), c, tc.modelURI, tc.explicitName)

			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				assert.Nil(t, spec)
				return
			}
			require.NoError(t, err)
			if tc.wantNil {
				assert.Nil(t, spec)
			} else {
				assert.NotNil(t, spec)
			}
		})
	}
}

func TestExtractCaBundleConfig(t *testing.T) {
	tests := []struct {
		name          string
		env           []corev1.EnvVar
		namespace     string
		wantCmName    string // configMapName the init container should mount
		wantSourceCm  string // sourceConfigMapName the CA bundle reconciler must copy
		wantMountPath string
	}{
		{
			name:          "no CA env at all → nothing to mount",
			env:           nil,
			namespace:     "user-ns",
			wantMountPath: constants.DefaultCaBundleVolumeMountPath,
		},
		{
			name: "CSC-set CA_BUNDLE_CONFIGMAP_NAME in user namespace → mount local copy, trigger reconciler",
			env: []corev1.EnvVar{
				{Name: constants.CaBundleConfigMapNameEnvVarKey, Value: "src"},
				{Name: constants.CaBundleVolumeMountPathEnvVarKey, Value: "/etc/ca"},
			},
			namespace:     "user-ns",
			wantCmName:    constants.DefaultGlobalCaBundleConfigMapName,
			wantSourceCm:  "src",
			wantMountPath: "/etc/ca",
		},
		{
			name: "CSC-set CA_BUNDLE_CONFIGMAP_NAME in kserve namespace → mount source directly, no copy",
			env: []corev1.EnvVar{
				{Name: constants.CaBundleConfigMapNameEnvVarKey, Value: "src"},
			},
			namespace:     constants.KServeNamespace,
			wantCmName:    "src",
			wantMountPath: constants.DefaultCaBundleVolumeMountPath,
		},
		{
			name: "credential-builder AWS_CA_BUNDLE_CONFIG_MAP wins over CSC source",
			env: []corev1.EnvVar{
				{Name: constants.CaBundleConfigMapNameEnvVarKey, Value: "cross-src"},
				{Name: constants.CaBundleVolumeMountPathEnvVarKey, Value: "/etc/ca/cross"},
				{Name: s3.AWSCABundleConfigMap, Value: "same-ns"},
				{Name: s3.AWSCABundle, Value: "/etc/ca/same/tls.crt"},
			},
			namespace:     "user-ns",
			wantCmName:    "same-ns",
			wantMountPath: "/etc/ca/same",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			container := &corev1.Container{Env: tc.env}
			cfg := extractCaBundleConfig(container, tc.namespace)
			assert.Equal(t, tc.wantCmName, cfg.configMapName, "configMapName")
			assert.Equal(t, tc.wantSourceCm, cfg.sourceConfigMapName, "sourceConfigMapName")
			assert.Equal(t, tc.wantMountPath, cfg.volumeMountPath, "volumeMountPath")
		})
	}
}
