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

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"golang.org/x/net/context"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestGetSupportingRuntimes(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	namespace := "default"

	tfRuntime := "tf-runtime"
	sklearnRuntime := "sklearn-runtime"
	pmmlRuntime := "pmml-runtime"
	mlserverRuntimeMMS := "mlserver-runtime-mms"
	mlserverRuntime := "mlserver-runtime"
	xgboostRuntime := "xgboost-runtime"
	// clusterServingRuntimePrefix := "cluster-"
	tritonRuntime := "triton-runtime"
	testRuntime := "test-runtime"
	huggingfaceMultinodeRuntime := "huggingface-multinode-runtime"
	protocolV2 := constants.ProtocolV2
	protocolV1 := constants.ProtocolV1

	servingRuntimeSpecs := map[string]v1alpha1.ServingRuntimeSpec{
		tfRuntime: {
			SupportedModelFormats: []v1alpha1.SupportedModelFormat{
				{
					Name:       "tensorflow",
					Version:    proto.String("1"),
					AutoSelect: proto.Bool(true),
					Priority:   proto.Int32(1),
				},
			},
			ProtocolVersions: []constants.InferenceServiceProtocol{constants.ProtocolV1, constants.ProtocolV2},
			ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
				Containers: []corev1.Container{
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
					Priority:   proto.Int32(1),
				},
			},
			ProtocolVersions: []constants.InferenceServiceProtocol{constants.ProtocolV1, constants.ProtocolV2},
			ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
				Containers: []corev1.Container{
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
					Name:     "pmml",
					Version:  proto.String("4"),
					Priority: proto.Int32(1),
				},
			},
			ProtocolVersions: []constants.InferenceServiceProtocol{constants.ProtocolV1, constants.ProtocolV2},
			ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
				Containers: []corev1.Container{
					{
						Name:  "kserve-container",
						Image: pmmlRuntime + "-image:latest",
					},
				},
			},
			Disabled: proto.Bool(true),
		},
		mlserverRuntimeMMS: {
			SupportedModelFormats: []v1alpha1.SupportedModelFormat{
				{
					Name:       "sklearn",
					Version:    proto.String("0"),
					AutoSelect: proto.Bool(true),
					Priority:   proto.Int32(2),
				},
				{
					Name:       "xgboost",
					Version:    proto.String("1"),
					AutoSelect: proto.Bool(true),
					Priority:   proto.Int32(2),
				},
				{
					Name:       "lightgbm",
					Version:    proto.String("3"),
					AutoSelect: proto.Bool(true),
					Priority:   proto.Int32(2),
				},
			},
			ProtocolVersions: []constants.InferenceServiceProtocol{constants.ProtocolV2},
			ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
				Containers: []corev1.Container{
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
		mlserverRuntime: {
			SupportedModelFormats: []v1alpha1.SupportedModelFormat{
				{
					Name:       "sklearn",
					Version:    proto.String("0"),
					AutoSelect: proto.Bool(true),
					Priority:   proto.Int32(2),
				},
				{
					Name:       "lightgbm",
					Version:    proto.String("3"),
					AutoSelect: proto.Bool(true),
					Priority:   proto.Int32(2),
				},
			},
			ProtocolVersions: []constants.InferenceServiceProtocol{constants.ProtocolV2},
			ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
				Containers: []corev1.Container{
					{
						Name:  "kserve-container",
						Image: mlserverRuntime + "-image:latest",
					},
				},
			},
			Disabled:   proto.Bool(false),
			MultiModel: proto.Bool(false),
		},
		xgboostRuntime: {
			SupportedModelFormats: []v1alpha1.SupportedModelFormat{
				{
					Name:       "xgboost",
					Version:    proto.String("0"),
					AutoSelect: proto.Bool(true),
					Priority:   proto.Int32(1),
				},
			},
			ProtocolVersions: []constants.InferenceServiceProtocol{constants.ProtocolV2},
			ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
				Containers: []corev1.Container{
					{
						Name:  "kserve-container",
						Image: sklearnRuntime + "-image:latest",
					},
				},
			},
			Disabled: proto.Bool(false),
		},
		tritonRuntime: {
			SupportedModelFormats: []v1alpha1.SupportedModelFormat{
				{
					Name:       "sklearn",
					Version:    proto.String("0"),
					AutoSelect: proto.Bool(true),
				},
				{
					Name:       "triton",
					Version:    proto.String("1"),
					AutoSelect: proto.Bool(true),
					Priority:   proto.Int32(1),
				},
				{
					Name:       "lightgbm",
					Version:    proto.String("3"),
					AutoSelect: proto.Bool(true),
				},
			},
			ProtocolVersions: []constants.InferenceServiceProtocol{constants.ProtocolV2},
			ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
				Containers: []corev1.Container{
					{
						Name:  "kserve-container",
						Image: mlserverRuntime + "-image:latest",
					},
				},
			},
			Disabled:   proto.Bool(false),
			MultiModel: proto.Bool(false),
		},
		testRuntime: {
			SupportedModelFormats: []v1alpha1.SupportedModelFormat{
				{
					Name:       "sklearn",
					Version:    proto.String("0"),
					AutoSelect: proto.Bool(true),
				},
			},
			ProtocolVersions: []constants.InferenceServiceProtocol{constants.ProtocolV2},
			ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
				Containers: []corev1.Container{
					{
						Name:  "kserve-container",
						Image: mlserverRuntime + "-image:latest",
					},
				},
			},
			Disabled:   proto.Bool(false),
			MultiModel: proto.Bool(false),
		},
		huggingfaceMultinodeRuntime: {
			SupportedModelFormats: []v1alpha1.SupportedModelFormat{
				{
					Name:       "huggingface",
					Version:    proto.String("1"),
					AutoSelect: proto.Bool(true),
					Priority:   proto.Int32(2),
				},
			},
			ProtocolVersions: []constants.InferenceServiceProtocol{constants.ProtocolV1, constants.ProtocolV2},
			ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
				Containers: []corev1.Container{
					{
						Name:  "kserve-container",
						Image: huggingfaceMultinodeRuntime + "-image:latest",
					},
				},
			},
			WorkerSpec: &v1alpha1.WorkerSpec{},
			MultiModel: proto.Bool(false),
			Disabled:   proto.Bool(false),
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
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mlserverRuntime,
					Namespace: namespace,
				},
				Spec: servingRuntimeSpecs[mlserverRuntime],
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tritonRuntime,
					Namespace: namespace,
				},
				Spec: servingRuntimeSpecs[tritonRuntime],
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRuntime,
					Namespace: namespace,
				},
				Spec: servingRuntimeSpecs[testRuntime],
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      huggingfaceMultinodeRuntime,
					Namespace: namespace,
				},
				Spec: servingRuntimeSpecs[huggingfaceMultinodeRuntime],
			},
		},
	}

	// ODH does not support ClusterServingRuntimeList
	// clusterRuntimes := &v1alpha1.ClusterServingRuntimeList{
	//	Items: []v1alpha1.ClusterServingRuntime{
	//		{
	//			ObjectMeta: metav1.ObjectMeta{
	//				Name: clusterServingRuntimePrefix + mlserverRuntimeMMS,
	//			},
	//			Spec: servingRuntimeSpecs[mlserverRuntimeMMS],
	//		},
	//		{
	//			ObjectMeta: metav1.ObjectMeta{
	//				Name: clusterServingRuntimePrefix + tfRuntime,
	//			},
	//			Spec: servingRuntimeSpecs[tfRuntime],
	//		},
	//		{
	//			ObjectMeta: metav1.ObjectMeta{
	//				Name: clusterServingRuntimePrefix + xgboostRuntime,
	//			},
	//			Spec: servingRuntimeSpecs[xgboostRuntime],
	//		},
	//	 },
	// }

	storageUri := "s3://test/model"
	scenarios := map[string]struct {
		spec        *ModelSpec
		isMMS       bool
		isMultinode bool
		expected    []v1alpha1.SupportedRuntime
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
			isMMS:       false,
			isMultinode: false,
			expected:    []v1alpha1.SupportedRuntime{{Name: tfRuntime, Spec: servingRuntimeSpecs[tfRuntime]} /*, {Name: clusterServingRuntimePrefix + tfRuntime, Spec: servingRuntimeSpecs[tfRuntime]}*/},
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
			isMMS:       false,
			isMultinode: false,
			expected:    []v1alpha1.SupportedRuntime{},
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
			isMMS:       false,
			isMultinode: false,
			expected:    []v1alpha1.SupportedRuntime{},
		},
		"ModelMeshCompatibleRuntimeModelFormatSpecified": {
			spec: &ModelSpec{
				ModelFormat: ModelFormat{
					Name: "sklearn",
				},
				PredictorExtensionSpec: PredictorExtensionSpec{
					ProtocolVersion: &protocolV2,
					StorageURI:      &storageUri,
				},
			},
			isMMS:       true,
			isMultinode: false,
			expected:    []v1alpha1.SupportedRuntime{ /*{Name: clusterServingRuntimePrefix + mlserverRuntimeMMS, Spec: servingRuntimeSpecs[mlserverRuntimeMMS]}*/ },
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
			isMMS:       false,
			isMultinode: false,
			expected:    []v1alpha1.SupportedRuntime{{Name: sklearnRuntime, Spec: servingRuntimeSpecs[sklearnRuntime]}},
		},
		"RuntimeV2ProtocolSpecified": {
			spec: &ModelSpec{
				ModelFormat: ModelFormat{
					Name: "xgboost",
				},
				PredictorExtensionSpec: PredictorExtensionSpec{
					ProtocolVersion: &protocolV2,
					StorageURI:      &storageUri,
				},
			},
			isMMS:       false,
			isMultinode: false,
			expected:    []v1alpha1.SupportedRuntime{ /*{Name: clusterServingRuntimePrefix + xgboostRuntime, Spec: servingRuntimeSpecs[xgboostRuntime]}*/ },
		},
		"RuntimeV1ProtocolNotFound": {
			spec: &ModelSpec{
				ModelFormat: ModelFormat{
					Name: "xgboost",
				},
				PredictorExtensionSpec: PredictorExtensionSpec{
					ProtocolVersion: &protocolV1,
					StorageURI:      &storageUri,
				},
			},
			isMMS:       false,
			isMultinode: false,
			expected:    []v1alpha1.SupportedRuntime{},
		},
		"MultipleRuntimeSupportsModelFormatSpecified": {
			spec: &ModelSpec{
				ModelFormat: ModelFormat{
					Name: "sklearn",
				},
				PredictorExtensionSpec: PredictorExtensionSpec{
					ProtocolVersion: &protocolV2,
					StorageURI:      &storageUri,
				},
			},
			isMMS:       false,
			isMultinode: false,
			expected: []v1alpha1.SupportedRuntime{
				{Name: mlserverRuntime, Spec: servingRuntimeSpecs[mlserverRuntime]},
				{Name: sklearnRuntime, Spec: servingRuntimeSpecs[sklearnRuntime]},
				{Name: testRuntime, Spec: servingRuntimeSpecs[testRuntime]},
				{Name: tritonRuntime, Spec: servingRuntimeSpecs[tritonRuntime]},
			},
		},
		"MultiNodeWorkerSpecSpecified": {
			spec: &ModelSpec{
				ModelFormat: ModelFormat{
					Name: "huggingface",
				},
				PredictorExtensionSpec: PredictorExtensionSpec{
					ProtocolVersion: &protocolV2,
					StorageURI:      &storageUri,
				},
			},
			isMMS:       false,
			isMultinode: true,
			expected: []v1alpha1.SupportedRuntime{
				{Name: huggingfaceMultinodeRuntime, Spec: servingRuntimeSpecs[huggingfaceMultinodeRuntime]},
			},
		},
	}

	s := runtime.NewScheme()
	err := v1alpha1.AddToScheme(s)
	if err != nil {
		t.Errorf("unable to add scheme : %v", err)
	}

	mockClient := fake.NewClientBuilder().WithLists(runtimes /*, clusterRuntimes*/).WithScheme(s).Build()
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res, _ := scenario.spec.GetSupportingRuntimes(context.Background(), mockClient, namespace, scenario.isMMS, scenario.isMultinode)
			if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", res, scenario.expected)
			}
		})
	}
}

func TestModelPredictorGetContainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	storageUri := "s3://test/model"
	isvcConfig := &InferenceServicesConfig{}
	objectMeta := metav1.ObjectMeta{
		Name:      "foo",
		Namespace: "default",
	}
	componentSpec := &ComponentExtensionSpec{
		MinReplicas: ptr.To(int32(3)),
		MaxReplicas: 2,
	}
	scenarios := map[string]struct {
		spec     *ModelSpec
		expected corev1.Container
	}{
		"ContainerSpecified": {
			spec: &ModelSpec{
				ModelFormat: ModelFormat{
					Name: "tensorflow",
				},
				PredictorExtensionSpec: PredictorExtensionSpec{
					StorageURI: &storageUri,
					Container: corev1.Container{
						Name: "foo",
						Env: []corev1.EnvVar{
							{
								Name:  "STORAGE_URI",
								Value: storageUri,
							},
						},
					},
				},
			},
			expected: corev1.Container{
				Name: "foo",
				Env: []corev1.EnvVar{
					{
						Name:  "STORAGE_URI",
						Value: storageUri,
					},
				},
			},
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			container := scenario.spec.GetContainer(objectMeta, componentSpec, isvcConfig)
			g.Expect(*container).To(gomega.Equal(scenario.expected))
		})
	}
}

func TestModelPredictorGetProtocol(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		spec    *ModelSpec
		matcher types.GomegaMatcher
	}{
		"DefaultProtocol": {
			spec: &ModelSpec{
				ModelFormat: ModelFormat{
					Name: "tensorflow",
				},
				PredictorExtensionSpec: PredictorExtensionSpec{
					StorageURI: proto.String("s3://test/model"),
				},
			},
			matcher: gomega.Equal(constants.ProtocolV1),
		},
		"ProtocolV2Specified": {
			spec: &ModelSpec{
				ModelFormat: ModelFormat{
					Name: "tensorflow",
				},
				PredictorExtensionSpec: PredictorExtensionSpec{
					StorageURI:      proto.String("s3://test/model"),
					ProtocolVersion: (*constants.InferenceServiceProtocol)(proto.String(string(constants.ProtocolV2))),
				},
			},
			matcher: gomega.Equal(constants.ProtocolV2),
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			protocol := scenario.spec.GetProtocol()
			g.Expect(protocol).To(scenario.matcher)
		})
	}
}
