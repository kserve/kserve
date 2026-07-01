/*
Copyright 2024 The KServe Authors.

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
package components

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestComputeMpNodeAndGPUs(t *testing.T) {
	tests := []struct {
		name                 string
		pipelineParallelSize int
		tensorParallelSize   int
		expectedNodeCount    int
		expectedWorkerGPU    int
		expectedHeadGPU      int
	}{
		{
			name:                 "PP=2, TP=2: 2 nodes, 2 GPUs per node",
			pipelineParallelSize: 2,
			tensorParallelSize:   2,
			expectedNodeCount:    2,
			expectedWorkerGPU:    2,
			expectedHeadGPU:      2,
		},
		{
			name:                 "PP=4, TP=8: 4 nodes, 8 GPUs per node",
			pipelineParallelSize: 4,
			tensorParallelSize:   8,
			expectedNodeCount:    4,
			expectedWorkerGPU:    8,
			expectedHeadGPU:      8,
		},
		{
			name:                 "PP=1, TP=4: 1 node, 4 GPUs",
			pipelineParallelSize: 1,
			tensorParallelSize:   4,
			expectedNodeCount:    1,
			expectedWorkerGPU:    4,
			expectedHeadGPU:      4,
		},
		{
			name:                 "PP=2, TP=1: 2 nodes, 1 GPU per node",
			pipelineParallelSize: 2,
			tensorParallelSize:   1,
			expectedNodeCount:    2,
			expectedWorkerGPU:    1,
			expectedHeadGPU:      1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeCount, workerGPU, headGPU := computeMpNodeAndGPUs(tt.pipelineParallelSize, tt.tensorParallelSize)
			assert.Equal(t, tt.expectedNodeCount, nodeCount)
			assert.Equal(t, tt.expectedWorkerGPU, workerGPU)
			assert.Equal(t, tt.expectedHeadGPU, headGPU)
		})
	}
}

func TestAddStorageInitializerAnnotationsOciNative(t *testing.T) {
	// oci+native:// must pass ValidateStorageURI (it's in SupportedStorageURIPrefixList)
	// and set StorageInitializerSourceUriInternalAnnotationKey so InjectModelcar can fire.
	s := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add v1alpha1 to scheme: %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()
	p := &Predictor{client: fakeClient}

	ociNativeURI := constants.OciNativeURIPrefix + "ghcr.io/kserve/oci-native-test-fixture:v1"
	model := &v1beta1.ModelSpec{
		PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
			StorageURI: &ociNativeURI,
		},
	}
	annotations := map[string]string{}

	err := p.addStorageInitializerAnnotations(context.Background(), model, annotations, nil)
	assert.NoError(t, err, "oci+native:// must pass ValidateStorageURI without error")
	annotationVal, hasAnnotation := annotations[constants.StorageInitializerSourceUriInternalAnnotationKey]
	assert.True(t, hasAnnotation, "oci+native:// must set StorageInitializerSourceUriInternalAnnotationKey so InjectModelcar can inject the ImageVolume")
	assert.Equal(t, ociNativeURI, annotationVal)
}
