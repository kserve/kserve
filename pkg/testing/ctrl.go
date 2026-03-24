/*
Copyright 2023 The KServe Authors.

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

package testing

import (
	"path/filepath"

	routev1 "github.com/openshift/api/route/v1"
	istioclientv1 "istio.io/client-go/pkg/apis/networking/v1"

	kservescheme "github.com/kserve/kserve/pkg/scheme"
)

// NewEnvTest prepares k8s EnvTest with prereq
func NewEnvTest(options ...Option) *Config {
	testCRDs := WithCRDs(
		filepath.Join(ProjectRoot(), "test", "crds"),
	)
	schemes := WithScheme(kservescheme.AddAll, routev1.AddToScheme, istioclientv1.AddToScheme)

	return Configure(append(options, testCRDs, schemes)...)
}
