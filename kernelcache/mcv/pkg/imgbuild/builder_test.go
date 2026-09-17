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

package imgbuild

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew_BuildahAvailable(t *testing.T) {
	origHasApp := HasApp
	defer func() { HasApp = origHasApp }()

	HasApp = func(tool string) bool {
		return tool == Buildah
	}

	builder, err := New()
	assert.NoError(t, err)
	assert.IsType(t, &buildahBuilder{}, builder)
}

func TestNew_DockerFallback(t *testing.T) {
	origHasApp := HasApp
	defer func() { HasApp = origHasApp }()

	HasApp = func(tool string) bool {
		return tool == Docker
	}

	builder, err := New()
	assert.NoError(t, err)
	assert.IsType(t, &dockerBuilder{}, builder)
}

func TestNew_Unsupported(t *testing.T) {
	origHasApp := HasApp
	defer func() { HasApp = origHasApp }()

	HasApp = func(tool string) bool {
		return false
	}

	builder, err := New()
	assert.Nil(t, builder)
	assert.Error(t, err)
}

func TestNewWithBuilder_Buildah(t *testing.T) {
	origHasApp := HasApp
	defer func() { HasApp = origHasApp }()

	HasApp = func(tool string) bool {
		return tool == Buildah
	}

	builder, err := NewWithBuilder(Buildah)
	assert.NoError(t, err)
	assert.IsType(t, &buildahBuilder{}, builder)
}

func TestNewWithBuilder_Docker(t *testing.T) {
	origHasApp := HasApp
	defer func() { HasApp = origHasApp }()

	HasApp = func(tool string) bool {
		return tool == Docker
	}

	builder, err := NewWithBuilder(Docker)
	assert.NoError(t, err)
	assert.IsType(t, &dockerBuilder{}, builder)
}

func TestNewWithBuilder_Unsupported(t *testing.T) {
	builder, err := NewWithBuilder("unsupported")
	assert.Nil(t, builder)
	assert.Error(t, err)
}

func TestNewWithBuilder_AutoDetect(t *testing.T) {
	origHasApp := HasApp
	defer func() { HasApp = origHasApp }()

	HasApp = func(tool string) bool {
		return tool == Docker
	}

	builder, err := NewWithBuilder("")
	assert.NoError(t, err)
	assert.IsType(t, &dockerBuilder{}, builder)
}
