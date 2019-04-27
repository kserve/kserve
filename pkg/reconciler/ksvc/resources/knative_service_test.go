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
	"github.com/google/go-cmp/cmp"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/frameworks/tensorflow"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
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

var ksvcConfiguration = knservingv1alpha1.ConfigurationSpec{
	RevisionTemplate: knservingv1alpha1.RevisionTemplateSpec{
		Spec: knservingv1alpha1.RevisionSpec{
			Container: v1.Container{
				Image:   tensorflow.TensorflowServingImageName + ":" + kfsvc.Spec.Default.Tensorflow.RuntimeVersion,
				Command: []string{tensorflow.TensorflowEntrypointCommand},
				Args: []string{
					"--port=" + tensorflow.TensorflowServingGRPCPort,
					"--rest_api_port=" + tensorflow.TensorflowServingRestPort,
					"--model_name=mnist",
					"--model_base_path=" + kfsvc.Spec.Default.Tensorflow.ModelURI,
				},
			},
		},
	},
}

var ksvcCanaryConfiguration = knservingv1alpha1.ConfigurationSpec{
	RevisionTemplate: knservingv1alpha1.RevisionTemplateSpec{
		Spec: knservingv1alpha1.RevisionSpec{
			Container: v1.Container{
				Image:   tensorflow.TensorflowServingImageName + ":" + kfsvc.Spec.Default.Tensorflow.RuntimeVersion,
				Command: []string{tensorflow.TensorflowEntrypointCommand},
				Args: []string{
					"--port=" + tensorflow.TensorflowServingGRPCPort,
					"--rest_api_port=" + tensorflow.TensorflowServingRestPort,
					"--model_name=mnist",
					"--model_base_path=s3://test/mnist-2/export",
				},
			},
		},
	},
}

func TestKnativeServiceSpec(t *testing.T) {
	scenarios := map[string]struct {
		kfService    *v1alpha1.KFService
		expectedSpec *knservingv1alpha1.Service
		shouldFail   bool
	}{
		"RunLatestModel": {
			kfService: kfsvc,
			expectedSpec: &knservingv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ServiceSpec{
					Release: &knservingv1alpha1.ReleaseType{
						Revisions:     []string{"@latest"},
						Configuration: ksvcConfiguration,
					},
				},
			},
		},
		"CanaryRollback": {
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
						ModelSpec: v1alpha1.ModelSpec{
							Tensorflow: &v1alpha1.TensorflowSpec{
								ModelURI:       "s3://test/mnist-2/export",
								RuntimeVersion: "1.13",
							},
						},
						TrafficPercent: 0,
					},
				},
				Status: v1alpha1.KFServiceStatus{
					Default: v1alpha1.StatusConfigurationSpec{
						Name: "v1",
					},
				},
			},
			expectedSpec: &knservingv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ServiceSpec{
					Release: &knservingv1alpha1.ReleaseType{
						Revisions:     []string{"@latest"},
						Configuration: ksvcConfiguration,
					},
				},
			},
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
			expectedSpec: &knservingv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ServiceSpec{
					Release: &knservingv1alpha1.ReleaseType{
						Revisions:      []string{"v1", "@latest"},
						RolloutPercent: 20,
						Configuration:  ksvcCanaryConfiguration,
					},
				},
			},
		},
		"PromoteStableModel": {
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
							ModelURI:       "s3://test/mnist-2/export",
							RuntimeVersion: "1.13",
						},
					},
					Canary: &v1alpha1.CanarySpec{
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
			expectedSpec: &knservingv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ServiceSpec{
					Release: &knservingv1alpha1.ReleaseType{
						Revisions:     []string{"@latest"},
						Configuration: ksvcCanaryConfiguration,
					},
				},
			},
		},
		"CanaryIncreasePercent": {
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
						ModelSpec: v1alpha1.ModelSpec{
							Tensorflow: &v1alpha1.TensorflowSpec{
								ModelURI:       "s3://test/mnist-2/export",
								RuntimeVersion: "1.13",
							},
						},
						TrafficPercent: 50,
					},
				},
				Status: v1alpha1.KFServiceStatus{
					Default: v1alpha1.StatusConfigurationSpec{
						Name:    "v1",
						Traffic: 80,
					},
					Canary: v1alpha1.StatusConfigurationSpec{
						Name:    "v2",
						Traffic: 20,
					},
				},
			},
			expectedSpec: &knservingv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ServiceSpec{
					Release: &knservingv1alpha1.ReleaseType{
						Revisions:      []string{"v1", "@latest"},
						Configuration:  ksvcCanaryConfiguration,
						RolloutPercent: 50,
					},
				},
			},
		},
		"RunScikitModel": {
			kfService: &v1alpha1.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scikit",
					Namespace: "default",
				},
				Spec: v1alpha1.KFServiceSpec{
					MinReplicas: 1,
					MaxReplicas: 3,
					Default: v1alpha1.ModelSpec{
						ScikitLearn: &v1alpha1.ScikitLearnSpec{
							ModelURI:       "s3://test/scikit/export",
							RuntimeVersion: "1.0",
						},
					},
				},
			},
			expectedSpec: &knservingv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scikit",
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ServiceSpec{
					Release: &knservingv1alpha1.ReleaseType{
						Revisions: []string{"@latest"},
						Configuration: knservingv1alpha1.ConfigurationSpec{
							RevisionTemplate: knservingv1alpha1.RevisionTemplateSpec{
								Spec: knservingv1alpha1.RevisionSpec{
									Container: v1.Container{
										//TODO(@yuzisun) fill in once scikit is implemented
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		service, err := CreateKnativeService(scenario.kfService)
		// Validate
		if scenario.shouldFail && err == nil {
			t.Errorf("Test %q failed: returned success but expected error", name)
		}
		if !scenario.shouldFail {
			if err != nil {
				t.Errorf("Test %q failed: returned error: %v", name, err)
			}
			if diff := cmp.Diff(scenario.expectedSpec, service); diff != "" {
				t.Errorf("Test %q unexpected ServiceSpec (-want +got): %v", name, diff)
			}
		}
	}
}
