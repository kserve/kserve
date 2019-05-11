/*
Copyright 2019 kubeflow.org.

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

package resources

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var kfsvc = &v1alpha1.KFService{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "mnist",
		Namespace: "default",
	},
	Spec: v1alpha1.KFServiceSpec{
		MinReplicas: 1,
		MaxReplicas: 3,
		Default: v1alpha1.ModelSpec{
			Tensorflow: &v1alpha1.TensorflowSpec{
				ModelURI:       "s3://test/mnist/export",
				RuntimeVersion: "1.13",
			},
		},
	},
}

var defaultConfiguration = knservingv1alpha1.Configuration{
	ObjectMeta: metav1.ObjectMeta{
		Name:        constants.DefaultConfigurationName("mnist"),
		Namespace:   "default",
		Annotations: map[string]string{"autoscaling.knative.dev/maxScale": "3", "autoscaling.knative.dev/minScale": "1"},
	},
	Spec: knservingv1alpha1.ConfigurationSpec{
		RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"serving.kubeflow.org/kfservice": "mnist"},
			},
			Spec: knservingv1alpha1.RevisionSpec{
				Container: &v1.Container{
					Image:   v1alpha1.TensorflowServingImageName + ":" + kfsvc.Spec.Default.Tensorflow.RuntimeVersion,
					Command: []string{v1alpha1.TensorflowEntrypointCommand},
					Args: []string{
						"--port=" + v1alpha1.TensorflowServingGRPCPort,
						"--rest_api_port=" + v1alpha1.TensorflowServingRestPort,
						"--model_name=mnist",
						"--model_base_path=" + kfsvc.Spec.Default.Tensorflow.ModelURI,
					},
				},
			},
		},
	},
}

var canaryConfiguration = knservingv1alpha1.Configuration{
	ObjectMeta: metav1.ObjectMeta{
		Name:        constants.CanaryConfigurationName("mnist"),
		Namespace:   "default",
		Annotations: map[string]string{"autoscaling.knative.dev/maxScale": "3", "autoscaling.knative.dev/minScale": "1"},
	},
	Spec: knservingv1alpha1.ConfigurationSpec{
		RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"serving.kubeflow.org/kfservice": "mnist"},
			},
			Spec: knservingv1alpha1.RevisionSpec{
				Container: &v1.Container{
					Image:   v1alpha1.TensorflowServingImageName + ":" + kfsvc.Spec.Default.Tensorflow.RuntimeVersion,
					Command: []string{v1alpha1.TensorflowEntrypointCommand},
					Args: []string{
						"--port=" + v1alpha1.TensorflowServingGRPCPort,
						"--rest_api_port=" + v1alpha1.TensorflowServingRestPort,
						"--model_name=mnist",
						"--model_base_path=s3://test/mnist-2/export",
					},
				},
			},
		},
	},
}

func TestKnativeConfiguration(t *testing.T) {
	scenarios := map[string]struct {
		kfService                       *v1alpha1.KFService
		expectedDefault, expectedCanary *knservingv1alpha1.Configuration
		shouldFail                      bool
	}{
		"RunLatestModel": {
			kfService:       kfsvc,
			expectedDefault: &defaultConfiguration,
			expectedCanary:  nil,
		},
		"RunCanaryModel": {
			kfService: &v1alpha1.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: v1alpha1.KFServiceSpec{
					MinReplicas: 1,
					MaxReplicas: 3,
					Default: v1alpha1.ModelSpec{
						Tensorflow: &v1alpha1.TensorflowSpec{
							ModelURI:       "s3://test/mnist/export",
							RuntimeVersion: "1.13",
						},
					},
					Canary: &v1alpha1.CanarySpec{
						TrafficPercent: 20,
						ModelSpec: v1alpha1.ModelSpec{
							Tensorflow: &v1alpha1.TensorflowSpec{
								ModelURI:       "s3://test/mnist-2/export",
								RuntimeVersion: "1.13",
							},
						},
					},
				},
				Status: v1alpha1.KFServiceStatus{
					Default: v1alpha1.StatusConfigurationSpec{
						Name: "v1",
					},
				},
			},
			expectedDefault: &defaultConfiguration,
			expectedCanary:  &canaryConfiguration,
		},
		"RunScikitModel": {
			kfService: &v1alpha1.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scikit",
					Namespace: "default",
				},
				Spec: v1alpha1.KFServiceSpec{
					Default: v1alpha1.ModelSpec{
						ScikitLearn: &v1alpha1.ScikitLearnSpec{
							ModelURI:       "s3://test/scikit/export",
							RuntimeVersion: "1.0",
						},
					},
				},
			},
			expectedDefault: &knservingv1alpha1.Configuration{
				ObjectMeta: metav1.ObjectMeta{
					Name:        constants.DefaultConfigurationName("scikit"),
					Namespace:   "default",
					Annotations: map[string]string{},
				},
				Spec: knservingv1alpha1.ConfigurationSpec{
					RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"serving.kubeflow.org/kfservice": "scikit"},
						},
						Spec: knservingv1alpha1.RevisionSpec{
							Container: &v1.Container{
								Image: "notimplementedyet",
							},
						},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		defaultConfiguration, canaryConfiguration := CreateKnativeConfiguration(scenario.kfService)
		// Validate
		if scenario.shouldFail {
			t.Errorf("Test %q failed: returned success but expected error", name)
		} else {
			if diff := cmp.Diff(scenario.expectedDefault, defaultConfiguration); diff != "" {
				t.Errorf("Test %q unexpected default configuration (-want +got): %v", name, diff)
			}

			if diff := cmp.Diff(scenario.expectedCanary, canaryConfiguration); diff != "" {
				t.Errorf("Test %q unexpected canary configuration (-want +got): %v", name, diff)
			}
		}

	}
}
