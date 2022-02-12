/*
Copyright 2021 The KServe Authors.

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

package v1beta1

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetSupportingRuntimes(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	namespace := "default"

	tfRuntime := "tf-runtime"
	sklearnRuntime := "sklearn-runtime"
	pmmlRuntime := "pmml-runtime"
	mlserverRuntime := "mlserver-runtime"

	servingRuntimeSpecs := map[string]v1alpha1.ServingRuntimeSpec{
		tfRuntime: {
			SupportedModelFormats: []v1alpha1.SupportedModelFormat{
				{
					Name:       "tensorflow",
					Version:    proto.String("1"),
					AutoSelect: proto.Bool(true),
				},
			},
			ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
				Containers: []v1alpha1.Container{
					{
						Name:  "kserve-container",
						Image: tfRuntime + "-image:latest",
					},
				},
			},
			Disabled: proto.Bool(false),
		},
		sklearnRuntime: {
			SupportedModelFormats: []v1alpha1.SupportedModelFormat{
				{
					Name:       "sklearn",
					Version:    proto.String("0"),
					AutoSelect: proto.Bool(true),
				},
			},
			ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
				Containers: []v1alpha1.Container{
					{
						Name:  "kserve-container",
						Image: sklearnRuntime + "-image:latest",
					},
				},
			},
			Disabled: proto.Bool(false),
		},
		pmmlRuntime: {
			SupportedModelFormats: []v1alpha1.SupportedModelFormat{
				{
					Name:    "pmml",
					Version: proto.String("4"),
				},
			},
			ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
				Containers: []v1alpha1.Container{
					{
						Name:  "kserve-container",
						Image: pmmlRuntime + "-image:latest",
					},
				},
			},
			Disabled: proto.Bool(true),
		},
		mlserverRuntime: {
			SupportedModelFormats: []v1alpha1.SupportedModelFormat{
				{
					Name:       "sklearn",
					Version:    proto.String("0"),
					AutoSelect: proto.Bool(true),
				},
				{
					Name:    "xgboost",
					Version: proto.String("1"),
				},
				{
					Name:    "lightgbm",
					Version: proto.String("3"),
				},
			},
			ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
				Containers: []v1alpha1.Container{
					{
						Name:  "kserve-container",
						Image: pmmlRuntime + "-image:latest",
					},
				},
			},
			GrpcMultiModelManagementEndpoint: proto.String("port:8085"),
			Disabled:                         proto.Bool(false),
			MultiModel:                       proto.Bool(true),
		},
	}

	runtimes := &v1alpha1.ServingRuntimeList{
		Items: []v1alpha1.ServingRuntime{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tfRuntime,
					Namespace: namespace,
				},
				Spec: servingRuntimeSpecs[tfRuntime],
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sklearnRuntime,
					Namespace: namespace,
				},
				Spec: servingRuntimeSpecs[sklearnRuntime],
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pmmlRuntime,
					Namespace: namespace,
				},
				Spec: servingRuntimeSpecs[pmmlRuntime],
			},
		},
	}

	clusterRuntimes := &v1alpha1.ClusterServingRuntimeList{
		Items: []v1alpha1.ClusterServingRuntime{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: mlserverRuntime,
				},
				Spec: servingRuntimeSpecs[mlserverRuntime],
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: tfRuntime,
				},
				Spec: servingRuntimeSpecs[tfRuntime],
			},
		},
	}

	var storageUri = "s3://test/model"
	scenarios := map[string]struct {
		spec     *ModelSpec
		isMMS    bool
		expected []v1alpha1.ServingRuntimeSpec
	}{
		"BothClusterAndNamespaceRuntimesSupportModel": {
			spec: &ModelSpec{
				ModelFormat: ModelFormat{
					Name: "tensorflow",
				},
				PredictorExtensionSpec: PredictorExtensionSpec{
					StorageURI: &storageUri,
				},
			},
			isMMS:    false,
			expected: []v1alpha1.ServingRuntimeSpec{servingRuntimeSpecs[tfRuntime], servingRuntimeSpecs[tfRuntime]},
		},
		"RuntimeNotFound": {
			spec: &ModelSpec{
				ModelFormat: ModelFormat{
					Name: "nonexistent-modelformat",
				},
				PredictorExtensionSpec: PredictorExtensionSpec{
					StorageURI: &storageUri,
				},
			},
			isMMS:    false,
			expected: []v1alpha1.ServingRuntimeSpec{},
		},
		"ModelFormatWithDisabledRuntimeSpecified": {
			spec: &ModelSpec{
				ModelFormat: ModelFormat{
					Name: "pmml",
				},
				PredictorExtensionSpec: PredictorExtensionSpec{
					StorageURI: &storageUri,
				},
			},
			isMMS:    false,
			expected: []v1alpha1.ServingRuntimeSpec{},
		},
		"ModelMeshCompatibleRuntimeModelFormatSpecified": {
			spec: &ModelSpec{
				ModelFormat: ModelFormat{
					Name: "sklearn",
				},
				PredictorExtensionSpec: PredictorExtensionSpec{
					StorageURI: &storageUri,
				},
			},
			isMMS:    true,
			expected: []v1alpha1.ServingRuntimeSpec{servingRuntimeSpecs[mlserverRuntime]},
		},
		"SMSRuntimeModelFormatSpecified": {
			spec: &ModelSpec{
				ModelFormat: ModelFormat{
					Name: "sklearn",
				},
				PredictorExtensionSpec: PredictorExtensionSpec{
					StorageURI: &storageUri,
				},
			},
			isMMS:    false,
			expected: []v1alpha1.ServingRuntimeSpec{servingRuntimeSpecs[sklearnRuntime]},
		},
	}

	s := runtime.NewScheme()
	v1alpha1.AddToScheme(s)

	mockClient := fake.NewClientBuilder().WithLists(runtimes, clusterRuntimes).WithScheme(s).Build()
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res, _ := scenario.spec.GetSupportingRuntimes(mockClient, namespace, scenario.isMMS)
			if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", res, scenario.expected)
			}
		})
	}

}
