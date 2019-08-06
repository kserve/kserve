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

package knative

import (
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	"testing"

	"github.com/google/go-cmp/cmp"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var kfsvc = v1alpha1.KFService{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "mnist",
		Namespace: "default",
		Annotations: map[string]string{
			constants.KFServiceGKEAcceleratorAnnotationKey: "nvidia-tesla-t4",
		},
	},
	Spec: v1alpha1.KFServiceSpec{
		Default: v1alpha1.ModelSpec{
			MinReplicas: 1,
			MaxReplicas: 3,
			Tensorflow: &v1alpha1.TensorflowSpec{
				ModelURI:       "s3://test/mnist/export",
				RuntimeVersion: "1.13.0",
			},
			ServiceAccountName: "testsvcacc",
		},
	},
}

var configMapData = map[string]string{
	"frameworks": `{
        "tensorflow" : {
            "image" : "tensorflow/tfserving"
        },
        "sklearn" : {
            "image" : "kfserving/sklearnserver"
        },
        "xgboost" : {
            "image" : "kfserving/xgbserver"
        }
    }`,
}

var defaultConfiguration = &knservingv1alpha1.Configuration{
	ObjectMeta: metav1.ObjectMeta{
		Name:      constants.DefaultConfigurationName("mnist"),
		Namespace: "default",
	},
	Spec: knservingv1alpha1.ConfigurationSpec{
		Template: &knservingv1alpha1.RevisionTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"serving.kubeflow.org/kfservice": "mnist"},
				Annotations: map[string]string{
					"autoscaling.knative.dev/class":                          "kpa.autoscaling.knative.dev",
					"autoscaling.knative.dev/target":                         "1",
					"autoscaling.knative.dev/minScale":                       "1",
					"autoscaling.knative.dev/maxScale":                       "3",
					constants.KFServiceGKEAcceleratorAnnotationKey:           "nvidia-tesla-t4",
					constants.ModelInitializerSourceUriInternalAnnotationKey: kfsvc.Spec.Default.Tensorflow.ModelURI,
				},
			},
			Spec: knservingv1alpha1.RevisionSpec{
				RevisionSpec: v1beta1.RevisionSpec{
					TimeoutSeconds: &constants.DefaultTimeout,
					PodSpec: v1.PodSpec{
						ServiceAccountName: "testsvcacc",
						Containers: []v1.Container{
							{
								Image:   v1alpha1.TensorflowServingImageName + ":" + kfsvc.Spec.Default.Tensorflow.RuntimeVersion,
								Command: []string{v1alpha1.TensorflowEntrypointCommand},
								Args: []string{
									"--port=" + v1alpha1.TensorflowServingGRPCPort,
									"--rest_api_port=" + v1alpha1.TensorflowServingRestPort,
									"--model_name=mnist",
									"--model_base_path=" + constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
				},
			},
		},
	},
}

var canaryConfiguration = &knservingv1alpha1.Configuration{
	ObjectMeta: metav1.ObjectMeta{
		Name:      constants.CanaryConfigurationName("mnist"),
		Namespace: "default",
	},
	Spec: knservingv1alpha1.ConfigurationSpec{
		Template: &knservingv1alpha1.RevisionTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"serving.kubeflow.org/kfservice": "mnist"},
				Annotations: map[string]string{
					"autoscaling.knative.dev/class":                          "kpa.autoscaling.knative.dev",
					"autoscaling.knative.dev/target":                         "1",
					"autoscaling.knative.dev/minScale":                       "1",
					"autoscaling.knative.dev/maxScale":                       "3",
					constants.KFServiceGKEAcceleratorAnnotationKey:           "nvidia-tesla-t4",
					constants.ModelInitializerSourceUriInternalAnnotationKey: "s3://test/mnist-2/export",
				},
			},
			Spec: knservingv1alpha1.RevisionSpec{
				RevisionSpec: v1beta1.RevisionSpec{
					TimeoutSeconds: &constants.DefaultTimeout,
					PodSpec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Image:   v1alpha1.TensorflowServingImageName + ":" + kfsvc.Spec.Default.Tensorflow.RuntimeVersion,
								Command: []string{v1alpha1.TensorflowEntrypointCommand},
								Args: []string{
									"--port=" + v1alpha1.TensorflowServingGRPCPort,
									"--rest_api_port=" + v1alpha1.TensorflowServingRestPort,
									"--model_name=mnist",
									"--model_base_path=" + constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
				},
			},
		},
	},
}

func TestKnativeConfiguration(t *testing.T) {
	scenarios := map[string]struct {
		configMapData   map[string]string
		kfService       v1alpha1.KFService
		expectedDefault *knservingv1alpha1.Configuration
		expectedCanary  *knservingv1alpha1.Configuration
	}{
		"RunLatestModel": {
			kfService:       kfsvc,
			expectedDefault: defaultConfiguration,
			expectedCanary:  nil,
		},
		"RunCanaryModel": {
			kfService: v1alpha1.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
					Annotations: map[string]string{
						constants.KFServiceGKEAcceleratorAnnotationKey: "nvidia-tesla-t4",
					},
				},
				Spec: v1alpha1.KFServiceSpec{
					Default: v1alpha1.ModelSpec{
						MinReplicas: 1,
						MaxReplicas: 3,
						Tensorflow: &v1alpha1.TensorflowSpec{
							ModelURI:       kfsvc.Spec.Default.Tensorflow.ModelURI,
							RuntimeVersion: "1.13.0",
						},
						ServiceAccountName: "testsvcacc",
					},
					CanaryTrafficPercent: 20,
					Canary: &v1alpha1.ModelSpec{
						MinReplicas: 1,
						MaxReplicas: 3,
						Tensorflow: &v1alpha1.TensorflowSpec{
							ModelURI:       "s3://test/mnist-2/export",
							RuntimeVersion: "1.13.0",
						},
					},
				},
				Status: v1alpha1.KFServiceStatus{
					Default: v1alpha1.StatusConfigurationSpec{
						Name: "v1",
					},
				},
			},
			expectedDefault: defaultConfiguration,
			expectedCanary:  canaryConfiguration,
		},
		"RunSklearnModel": {
			kfService: v1alpha1.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sklearn",
					Namespace: "default",
				},
				Spec: v1alpha1.KFServiceSpec{
					Default: v1alpha1.ModelSpec{
						SKLearn: &v1alpha1.SKLearnSpec{
							ModelURI:       "s3://test/sklearn/export",
							RuntimeVersion: "latest",
						},
					},
				},
			},
			expectedDefault: &knservingv1alpha1.Configuration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultConfigurationName("sklearn"),
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ConfigurationSpec{
					Template: &knservingv1alpha1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"serving.kubeflow.org/kfservice": "sklearn"},
							Annotations: map[string]string{
								constants.ModelInitializerSourceUriInternalAnnotationKey: "s3://test/sklearn/export",
								"autoscaling.knative.dev/class":                          "kpa.autoscaling.knative.dev",
								"autoscaling.knative.dev/target":                         "1",
							},
						},
						Spec: knservingv1alpha1.RevisionSpec{
							RevisionSpec: v1beta1.RevisionSpec{
								TimeoutSeconds: &constants.DefaultTimeout,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: v1alpha1.SKLearnServerImageName + ":" + v1alpha1.DefaultSKLearnRuntimeVersion,
											Args: []string{
												"--model_name=sklearn",
												"--model_dir=" + constants.DefaultModelLocalMountPath,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"RunXgboostModel": {
			kfService: v1alpha1.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "xgboost",
					Namespace: "default",
				},
				Spec: v1alpha1.KFServiceSpec{
					Default: v1alpha1.ModelSpec{
						XGBoost: &v1alpha1.XGBoostSpec{
							ModelURI:       "s3://test/xgboost/export",
							RuntimeVersion: "latest",
						},
					},
				},
			},
			expectedDefault: &knservingv1alpha1.Configuration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultConfigurationName("xgboost"),
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ConfigurationSpec{
					Template: &knservingv1alpha1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"serving.kubeflow.org/kfservice": "xgboost"},
							Annotations: map[string]string{
								constants.ModelInitializerSourceUriInternalAnnotationKey: "s3://test/xgboost/export",
								"autoscaling.knative.dev/class":                          "kpa.autoscaling.knative.dev",
								"autoscaling.knative.dev/target":                         "1",
							},
						},
						Spec: knservingv1alpha1.RevisionSpec{
							RevisionSpec: v1beta1.RevisionSpec{
								TimeoutSeconds: &constants.DefaultTimeout,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: v1alpha1.XGBoostServerImageName + ":" + v1alpha1.DefaultXGBoostRuntimeVersion,
											Args: []string{
												"--model_name=xgboost",
												"--model_dir=" + constants.DefaultModelLocalMountPath,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"TestConfigOverride": {
			configMapData: configMapData,
			kfService: v1alpha1.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "xgboost",
					Namespace: "default",
				},
				Spec: v1alpha1.KFServiceSpec{
					Default: v1alpha1.ModelSpec{
						XGBoost: &v1alpha1.XGBoostSpec{
							ModelURI:       "s3://test/xgboost/export",
							RuntimeVersion: "latest",
						},
					},
				},
			},
			expectedDefault: &knservingv1alpha1.Configuration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultConfigurationName("xgboost"),
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ConfigurationSpec{
					Template: &knservingv1alpha1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"serving.kubeflow.org/kfservice": "xgboost"},
							Annotations: map[string]string{
								constants.ModelInitializerSourceUriInternalAnnotationKey: "s3://test/xgboost/export",
								"autoscaling.knative.dev/class":                          "kpa.autoscaling.knative.dev",
								"autoscaling.knative.dev/target":                         "1",
							},
						},
						Spec: knservingv1alpha1.RevisionSpec{
							RevisionSpec: v1beta1.RevisionSpec{
								TimeoutSeconds: &constants.DefaultTimeout,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: "kfserving/xgbserver:" + v1alpha1.DefaultXGBoostRuntimeVersion,
											Args: []string{
												"--model_name=xgboost",
												"--model_dir=" + constants.DefaultModelLocalMountPath,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"TestAnnotation": {
			kfService: v1alpha1.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sklearn",
					Namespace: "default",
					Annotations: map[string]string{
						"sourceName":                       "srcName",
						"prop1":                            "val1",
						"autoscaling.knative.dev/minScale": "2",
						"autoscaling.knative.dev/target":   "2",
						constants.ModelInitializerSourceUriInternalAnnotationKey: "test",
						"kubectl.kubernetes.io/last-applied-configuration":       "test2",
					},
				},
				Spec: v1alpha1.KFServiceSpec{
					Default: v1alpha1.ModelSpec{
						SKLearn: &v1alpha1.SKLearnSpec{
							ModelURI:       "s3://test/sklearn/export",
							RuntimeVersion: "latest",
						},
						MinReplicas: 1,
					},
				},
			},
			expectedDefault: &knservingv1alpha1.Configuration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultConfigurationName("sklearn"),
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ConfigurationSpec{
					Template: &knservingv1alpha1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"serving.kubeflow.org/kfservice": "sklearn"},
							Annotations: map[string]string{
								constants.ModelInitializerSourceUriInternalAnnotationKey: "s3://test/sklearn/export",
								"autoscaling.knative.dev/class":                          "kpa.autoscaling.knative.dev",
								"autoscaling.knative.dev/target":                         "2",
								"autoscaling.knative.dev/minScale":                       "1",
								"sourceName":                                             "srcName",
								"prop1":                                                  "val1",
							},
						},
						Spec: knservingv1alpha1.RevisionSpec{
							RevisionSpec: v1beta1.RevisionSpec{
								TimeoutSeconds: &constants.DefaultTimeout,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: v1alpha1.SKLearnServerImageName + ":" + v1alpha1.DefaultSKLearnRuntimeVersion,
											Args: []string{
												"--model_name=sklearn",
												"--model_dir=" + constants.DefaultModelLocalMountPath,
											},
										},
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
		configurationBuilder := NewConfigurationBuilder(c, &v1.ConfigMap{
			Data: scenario.configMapData,
		})
		actualDefaultConfiguration, err := configurationBuilder.CreateKnativeConfiguration(
			constants.DefaultConfigurationName(scenario.kfService.Name),
			scenario.kfService.ObjectMeta,
			&scenario.kfService.Spec.Default,
		)
		if err != nil {
			t.Errorf("Test %q unexpected error %s", name, err.Error())
		}

		if diff := cmp.Diff(scenario.expectedDefault, actualDefaultConfiguration); diff != "" {
			t.Errorf("Test %q unexpected default configuration (-want +got): %v", name, diff)
		}

		if scenario.kfService.Spec.Canary != nil {
			actualCanaryConfiguration, err := configurationBuilder.CreateKnativeConfiguration(
				constants.CanaryConfigurationName(kfsvc.Name),
				scenario.kfService.ObjectMeta,
				scenario.kfService.Spec.Canary,
			)
			if err != nil {
				t.Errorf("Test %q unexpected error %s", name, err.Error())
			}
			if diff := cmp.Diff(scenario.expectedCanary, actualCanaryConfiguration); diff != "" {
				t.Errorf("Test %q unexpected canary configuration (-want +got): %v", name, diff)
			}
		}

	}
}
