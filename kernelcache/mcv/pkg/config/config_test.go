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

package config

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitialize_Defaults(t *testing.T) {
	tempDir := t.TempDir()

	cfg, err := Initialize(tempDir)
	assert.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, filepath.Clean(tempDir), cfg.ConfDir)
	assert.Equal(t, defaultNamespace, cfg.MCV.MCVNamespace)
	assert.Equal(t, defaultKubeConfig, cfg.MCV.KubeConfig)
	assert.True(t, *cfg.MCV.EnabledGPU)
	assert.False(t, *cfg.MCV.EnabledBaremetal)
}

func TestEnvironmentOverrides(t *testing.T) {
	t.Setenv("ENABLE_GPU", "false")
	t.Setenv("ENABLE_BAREMETAL", "true")
	t.Setenv("MCV_NAMESPACE", "custom-ns")
	t.Setenv("KUBE_CONFIG", "/path/to/kubeconfig")

	tempDir := t.TempDir()
	once = sync.Once{} // reset singleton
	cfg, err := Initialize(tempDir)
	assert.NoError(t, err)

	assert.False(t, *cfg.MCV.EnabledGPU)
	assert.True(t, *cfg.MCV.EnabledBaremetal)
	assert.Equal(t, "custom-ns", cfg.MCV.MCVNamespace)
	assert.Equal(t, "/path/to/kubeconfig", cfg.MCV.KubeConfig)
}

func TestSetters(t *testing.T) {
	once = sync.Once{}
	cfg, _ := Initialize(t.TempDir())

	SetEnabledGPU(false)
	SetEnabledBaremetal(true)
	SetKubeConfig("/new/kubeconfig")

	assert.False(t, *cfg.MCV.EnabledGPU)
	assert.True(t, *cfg.MCV.EnabledBaremetal)
	assert.Equal(t, "/new/kubeconfig", cfg.MCV.KubeConfig)
}

func TestGetters(t *testing.T) {
	once = sync.Once{}
	cfg, _ := Initialize(t.TempDir())

	cfg.MCV.EnabledGPU = boolPtr(true)
	cfg.MCV.EnabledBaremetal = boolPtr(false)

	assert.True(t, IsGPUEnabled())
	assert.False(t, IsBaremetalEnabled())
	assert.Equal(t, cfg.MCV.KubeConfig, KubeConfig())
}

func boolPtr(b bool) *bool {
	return &b
}
