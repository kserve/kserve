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
	"testing"

	"knative.dev/serving/pkg/apis/serving/v1beta1"

	"github.com/google/go-cmp/cmp"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	knservingv1alpha1 "knative.dev/serving/pkg/apis/serving/v1alpha1"
)

const (
	DefaultSKLearnRuntimeVersion       = "latest"
	DefaultTensorflowRuntimeVersionGPU = "latest-gpu"
	DefaultXGBoostRuntimeVersion       = "latest"
	TensorflowServingImageName         = "tensorflow/tfserving"
	SKLearnServerImageName             = "kfserving/sklearnserver"
	XGBoostServerImageName             = "kfserving/xgbserver"
)

var isvc = v1alpha2.InferenceService{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "mnist",
		Namespace: "default",
		Annotations: map[string]string{
			constants.InferenceServiceGKEAcceleratorAnnotationKey: "nvidia-tesla-t4",
		},
	},
	Spec: v1alpha2.InferenceServiceSpec{
		Default: v1alpha2.EndpointSpec{
			Predictor: v1alpha2.PredictorSpec{
				DeploymentSpec: v1alpha2.DeploymentSpec{
					MinReplicas:        1,
					MaxReplicas:        3,
					ServiceAccountName: "testsvcacc",
				},
				Tensorflow: &v1alpha2.TensorflowSpec{
					StorageURI:     "s3://test/mnist/export",
					RuntimeVersion: "1.13.0",
				},
			},
		},
	},
}

var configMapData = map[string]string{
	"predictors": `{
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

var defaultService = &knservingv1alpha1.Service{
	ObjectMeta: metav1.ObjectMeta{
		Name:      constants.DefaultPredictorServiceName("mnist"),
		Namespace: "default",
	},
	Spec: knservingv1alpha1.ServiceSpec{
		ConfigurationSpec: knservingv1alpha1.ConfigurationSpec{
			Template: &knservingv1alpha1.RevisionTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"serving.kubeflow.org/inferenceservice": "mnist"},
					Annotations: map[string]string{
						"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
						"autoscaling.knative.dev/target":                           "1",
						"autoscaling.knative.dev/minScale":                         "1",
						"autoscaling.knative.dev/maxScale":                         "3",
						constants.InferenceServiceGKEAcceleratorAnnotationKey:      "nvidia-tesla-t4",
						constants.StorageInitializerSourceUriInternalAnnotationKey: isvc.Spec.Default.Predictor.Tensorflow.StorageURI,
					},
				},
				Spec: knservingv1alpha1.RevisionSpec{
					RevisionSpec: v1beta1.RevisionSpec{
						TimeoutSeconds: &constants.DefaultTimeout,
						PodSpec: v1.PodSpec{
							ServiceAccountName: "testsvcacc",
							Containers: []v1.Container{
								{
									Image:   TensorflowServingImageName + ":" + isvc.Spec.Default.Predictor.Tensorflow.RuntimeVersion,
									Command: []string{v1alpha2.TensorflowEntrypointCommand},
									Args: []string{
										"--port=" + v1alpha2.TensorflowServingGRPCPort,
										"--rest_api_port=" + v1alpha2.TensorflowServingRestPort,
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
	},
}

var canaryService = &knservingv1alpha1.Service{
	ObjectMeta: metav1.ObjectMeta{
		Name:      constants.CanaryPredictorServiceName("mnist"),
		Namespace: "default",
	},
	Spec: knservingv1alpha1.ServiceSpec{
		ConfigurationSpec: knservingv1alpha1.ConfigurationSpec{
			Template: &knservingv1alpha1.RevisionTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"serving.kubeflow.org/inferenceservice": "mnist"},
					Annotations: map[string]string{
						"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
						"autoscaling.knative.dev/target":                           "1",
						"autoscaling.knative.dev/minScale":                         "1",
						"autoscaling.knative.dev/maxScale":                         "3",
						constants.InferenceServiceGKEAcceleratorAnnotationKey:      "nvidia-tesla-t4",
						constants.StorageInitializerSourceUriInternalAnnotationKey: "s3://test/mnist-2/export",
					},
				},
				Spec: knservingv1alpha1.RevisionSpec{
					RevisionSpec: v1beta1.RevisionSpec{
						TimeoutSeconds: &constants.DefaultTimeout,
						PodSpec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image:   TensorflowServingImageName + ":" + isvc.Spec.Default.Predictor.Tensorflow.RuntimeVersion,
									Command: []string{v1alpha2.TensorflowEntrypointCommand},
									Args: []string{
										"--port=" + v1alpha2.TensorflowServingGRPCPort,
										"--rest_api_port=" + v1alpha2.TensorflowServingRestPort,
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
	},
}

func TestInferenceServiceToKnativeService(t *testing.T) {
	scenarios := map[string]struct {
		inferenceService v1alpha2.InferenceService
		expectedDefault  *knservingv1alpha1.Service
		expectedCanary   *knservingv1alpha1.Service
	}{
		"RunLatestModel": {
			inferenceService: isvc,
			expectedDefault:  defaultService,
			expectedCanary:   nil,
		},
		"RunCanaryModel": {
			inferenceService: v1alpha2.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
					Annotations: map[string]string{
						constants.InferenceServiceGKEAcceleratorAnnotationKey: "nvidia-tesla-t4",
					},
				},
				Spec: v1alpha2.InferenceServiceSpec{
					Default: v1alpha2.EndpointSpec{
						Predictor: v1alpha2.PredictorSpec{
							DeploymentSpec: v1alpha2.DeploymentSpec{
								MinReplicas:        1,
								MaxReplicas:        3,
								ServiceAccountName: "testsvcacc",
							},
							Tensorflow: &v1alpha2.TensorflowSpec{
								StorageURI:     isvc.Spec.Default.Predictor.Tensorflow.StorageURI,
								RuntimeVersion: "1.13.0",
							},
						},
					},
					CanaryTrafficPercent: 20,
					Canary: &v1alpha2.EndpointSpec{
						Predictor: v1alpha2.PredictorSpec{
							DeploymentSpec: v1alpha2.DeploymentSpec{
								MinReplicas: 1,
								MaxReplicas: 3,
							},
							Tensorflow: &v1alpha2.TensorflowSpec{
								StorageURI:     "s3://test/mnist-2/export",
								RuntimeVersion: "1.13.0",
							},
						},
					},
				},
				Status: v1alpha2.InferenceServiceStatus{
					Default: &v1alpha2.EndpointStatusMap{
						constants.Predictor: &v1alpha2.StatusConfigurationSpec{
							Name: "v1",
						},
					},
				},
			},
			expectedDefault: defaultService,
			expectedCanary:  canaryService,
		},
		"RunSklearnModel": {
			inferenceService: v1alpha2.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sklearn",
					Namespace: "default",
				},
				Spec: v1alpha2.InferenceServiceSpec{
					Default: v1alpha2.EndpointSpec{
						Predictor: v1alpha2.PredictorSpec{
							SKLearn: &v1alpha2.SKLearnSpec{
								StorageURI:     "s3://test/sklearn/export",
								RuntimeVersion: "latest",
							},
						},
					},
				},
			},
			expectedDefault: &knservingv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultPredictorServiceName("sklearn"),
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ServiceSpec{
					ConfigurationSpec: knservingv1alpha1.ConfigurationSpec{
						Template: &knservingv1alpha1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kubeflow.org/inferenceservice": "sklearn"},
								Annotations: map[string]string{
									constants.StorageInitializerSourceUriInternalAnnotationKey: "s3://test/sklearn/export",
									"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/target":                           "1",
								},
							},
							Spec: knservingv1alpha1.RevisionSpec{
								RevisionSpec: v1beta1.RevisionSpec{
									TimeoutSeconds: &constants.DefaultTimeout,
									PodSpec: v1.PodSpec{
										Containers: []v1.Container{
											{
												Image: SKLearnServerImageName + ":" + DefaultSKLearnRuntimeVersion,
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
		},
		"RunXgboostModel": {
			inferenceService: v1alpha2.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "xgboost",
					Namespace: "default",
				},
				Spec: v1alpha2.InferenceServiceSpec{
					Default: v1alpha2.EndpointSpec{
						Predictor: v1alpha2.PredictorSpec{
							XGBoost: &v1alpha2.XGBoostSpec{
								StorageURI:     "s3://test/xgboost/export",
								RuntimeVersion: "latest",
							},
						},
					},
				},
			},
			expectedDefault: &knservingv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultPredictorServiceName("xgboost"),
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ServiceSpec{
					ConfigurationSpec: knservingv1alpha1.ConfigurationSpec{
						Template: &knservingv1alpha1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kubeflow.org/inferenceservice": "xgboost"},
								Annotations: map[string]string{
									constants.StorageInitializerSourceUriInternalAnnotationKey: "s3://test/xgboost/export",
									"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/target":                           "1",
								},
							},
							Spec: knservingv1alpha1.RevisionSpec{
								RevisionSpec: v1beta1.RevisionSpec{
									TimeoutSeconds: &constants.DefaultTimeout,
									PodSpec: v1.PodSpec{
										Containers: []v1.Container{
											{
												Image: XGBoostServerImageName + ":" + DefaultXGBoostRuntimeVersion,
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
		},
		"TestConfigOverride": {
			inferenceService: v1alpha2.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "xgboost",
					Namespace: "default",
				},
				Spec: v1alpha2.InferenceServiceSpec{
					Default: v1alpha2.EndpointSpec{
						Predictor: v1alpha2.PredictorSpec{
							XGBoost: &v1alpha2.XGBoostSpec{
								StorageURI:     "s3://test/xgboost/export",
								RuntimeVersion: "latest",
							},
						},
					},
				},
			},
			expectedDefault: &knservingv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultPredictorServiceName("xgboost"),
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ServiceSpec{
					ConfigurationSpec: knservingv1alpha1.ConfigurationSpec{
						Template: &knservingv1alpha1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kubeflow.org/inferenceservice": "xgboost"},
								Annotations: map[string]string{
									constants.StorageInitializerSourceUriInternalAnnotationKey: "s3://test/xgboost/export",
									"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/target":                           "1",
								},
							},
							Spec: knservingv1alpha1.RevisionSpec{
								RevisionSpec: v1beta1.RevisionSpec{
									TimeoutSeconds: &constants.DefaultTimeout,
									PodSpec: v1.PodSpec{
										Containers: []v1.Container{
											{
												Image: "kfserving/xgbserver:" + DefaultXGBoostRuntimeVersion,
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
		},
		"TestAnnotation": {
			inferenceService: v1alpha2.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sklearn",
					Namespace: "default",
					Annotations: map[string]string{
						"sourceName":                       "srcName",
						"prop1":                            "val1",
						"autoscaling.knative.dev/minScale": "2",
						"autoscaling.knative.dev/target":   "2",
						constants.StorageInitializerSourceUriInternalAnnotationKey: "test",
						"kubectl.kubernetes.io/last-applied-configuration":         "test2",
					},
				},
				Spec: v1alpha2.InferenceServiceSpec{
					Default: v1alpha2.EndpointSpec{
						Predictor: v1alpha2.PredictorSpec{
							SKLearn: &v1alpha2.SKLearnSpec{
								StorageURI:     "s3://test/sklearn/export",
								RuntimeVersion: "latest",
							},
							DeploymentSpec: v1alpha2.DeploymentSpec{
								MinReplicas: 1,
							},
						},
					},
				},
			},
			expectedDefault: &knservingv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultPredictorServiceName("sklearn"),
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ServiceSpec{
					ConfigurationSpec: knservingv1alpha1.ConfigurationSpec{
						Template: &knservingv1alpha1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kubeflow.org/inferenceservice": "sklearn"},
								Annotations: map[string]string{
									constants.StorageInitializerSourceUriInternalAnnotationKey: "s3://test/sklearn/export",
									"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/target":                           "2",
									"autoscaling.knative.dev/minScale":                         "1",
									"sourceName":                                               "srcName",
									"prop1":                                                    "val1",
								},
							},
							Spec: knservingv1alpha1.RevisionSpec{
								RevisionSpec: v1beta1.RevisionSpec{
									TimeoutSeconds: &constants.DefaultTimeout,
									PodSpec: v1.PodSpec{
										Containers: []v1.Container{
											{
												Image: SKLearnServerImageName + ":" + DefaultSKLearnRuntimeVersion,
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
		},
	}

	for name, scenario := range scenarios {
		serviceBuilder := NewServiceBuilder(c, &v1.ConfigMap{
			Data: configMapData,
		})
		actualDefaultService, err := serviceBuilder.CreatePredictorService(
			constants.DefaultPredictorServiceName(scenario.inferenceService.Name),
			scenario.inferenceService.ObjectMeta,
			&scenario.inferenceService.Spec.Default.Predictor,
		)
		if err != nil {
			t.Errorf("Test %q unexpected error %s", name, err.Error())
		}

		if diff := cmp.Diff(scenario.expectedDefault, actualDefaultService); diff != "" {
			t.Errorf("Test %q unexpected default service (-want +got): %v", name, diff)
		}

		if scenario.inferenceService.Spec.Canary != nil {
			actualCanaryService, err := serviceBuilder.CreatePredictorService(
				constants.CanaryPredictorServiceName(isvc.Name),
				scenario.inferenceService.ObjectMeta,
				&scenario.inferenceService.Spec.Canary.Predictor,
			)
			if err != nil {
				t.Errorf("Test %q unexpected error %s", name, err.Error())
			}
			if diff := cmp.Diff(scenario.expectedCanary, actualCanaryService); diff != "" {
				t.Errorf("Test %q unexpected canary service (-want +got): %v", name, diff)
			}
		}

	}
}

func TestTransformerToKnativeService(t *testing.T) {
	isvc := v1alpha2.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mnist",
			Namespace: "default",
		},
		Spec: v1alpha2.InferenceServiceSpec{
			Default: v1alpha2.EndpointSpec{
				Transformer: &v1alpha2.TransformerSpec{
					DeploymentSpec: v1alpha2.DeploymentSpec{
						MinReplicas:        1,
						MaxReplicas:        3,
						ServiceAccountName: "testsvcacc",
					},
					Custom: &v1alpha2.CustomSpec{
						Container: v1.Container{
							Image: "transformer:latest",
						},
					},
				},
				Predictor: v1alpha2.PredictorSpec{
					DeploymentSpec: v1alpha2.DeploymentSpec{
						MinReplicas:        1,
						MaxReplicas:        3,
						ServiceAccountName: "testsvcacc",
					},
					Tensorflow: &v1alpha2.TensorflowSpec{
						StorageURI:     "s3://test/mnist/export",
						RuntimeVersion: "1.13.0",
					},
				},
			},
		},
	}

	isvcCanary := isvc.DeepCopy()
	isvcCanary.Spec.CanaryTrafficPercent = 20
	isvcCanary.Spec.Canary = &v1alpha2.EndpointSpec{
		Transformer: &v1alpha2.TransformerSpec{
			DeploymentSpec: v1alpha2.DeploymentSpec{
				MinReplicas:        2,
				MaxReplicas:        4,
				ServiceAccountName: "testsvcacc",
			},
			Custom: &v1alpha2.CustomSpec{
				Container: v1.Container{
					Image: "transformer:v2",
				},
			},
		},
		Predictor: v1alpha2.PredictorSpec{
			DeploymentSpec: v1alpha2.DeploymentSpec{
				MinReplicas:        1,
				MaxReplicas:        3,
				ServiceAccountName: "testsvcacc",
			},
			Tensorflow: &v1alpha2.TensorflowSpec{
				StorageURI:     "s3://test/mnist-2/export",
				RuntimeVersion: "1.13.0",
			},
		},
	}

	var defaultService = &knservingv1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.DefaultTransformerServiceName("mnist"),
			Namespace: "default",
		},
		Spec: knservingv1alpha1.ServiceSpec{
			ConfigurationSpec: knservingv1alpha1.ConfigurationSpec{
				Template: &knservingv1alpha1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"serving.kubeflow.org/inferenceservice": "mnist"},
						Annotations: map[string]string{
							"autoscaling.knative.dev/class":    "kpa.autoscaling.knative.dev",
							"autoscaling.knative.dev/target":   "1",
							"autoscaling.knative.dev/minScale": "1",
							"autoscaling.knative.dev/maxScale": "3",
						},
					},
					Spec: knservingv1alpha1.RevisionSpec{
						RevisionSpec: v1beta1.RevisionSpec{
							TimeoutSeconds: &constants.DefaultTimeout,
							PodSpec: v1.PodSpec{
								ServiceAccountName: "testsvcacc",
								Containers: []v1.Container{
									{
										Image: "transformer:latest",
										Args: []string{
											constants.ArgumentModelName,
											isvc.Name,
											constants.ArgumentPredictorHost,
											constants.DefaultPredictorServiceName(isvc.Name) + "." + isvc.Namespace,
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

	var canaryService = &knservingv1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.CanaryTransformerServiceName("mnist"),
			Namespace: "default",
		},
		Spec: knservingv1alpha1.ServiceSpec{
			ConfigurationSpec: knservingv1alpha1.ConfigurationSpec{
				Template: &knservingv1alpha1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"serving.kubeflow.org/inferenceservice": "mnist"},
						Annotations: map[string]string{
							"autoscaling.knative.dev/class":    "kpa.autoscaling.knative.dev",
							"autoscaling.knative.dev/target":   "1",
							"autoscaling.knative.dev/minScale": "2",
							"autoscaling.knative.dev/maxScale": "4",
						},
					},
					Spec: knservingv1alpha1.RevisionSpec{
						RevisionSpec: v1beta1.RevisionSpec{
							TimeoutSeconds: &constants.DefaultTimeout,
							PodSpec: v1.PodSpec{
								ServiceAccountName: "testsvcacc",
								Containers: []v1.Container{
									{
										Image: "transformer:v2",
										Args: []string{
											constants.ArgumentModelName,
											isvc.Name,
											constants.ArgumentPredictorHost,
											constants.CanaryPredictorServiceName(isvc.Name) + "." + isvc.Namespace,
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

	scenarios := map[string]struct {
		inferenceService v1alpha2.InferenceService
		expectedDefault  *knservingv1alpha1.Service
		expectedCanary   *knservingv1alpha1.Service
	}{
		"RunLatestModel": {
			inferenceService: isvc,
			expectedDefault:  defaultService,
			expectedCanary:   nil,
		},
		"RunCanaryModel": {
			inferenceService: *isvcCanary,
			expectedDefault:  defaultService,
			expectedCanary:   canaryService,
		},
	}

	for name, scenario := range scenarios {
		serviceBuilder := NewServiceBuilder(c, &v1.ConfigMap{
			Data: configMapData,
		})
		actualDefaultService, err := serviceBuilder.CreateTransformerService(
			constants.DefaultTransformerServiceName(scenario.inferenceService.Name),
			scenario.inferenceService.ObjectMeta,
			scenario.inferenceService.Spec.Default.Transformer, false)
		if err != nil {
			t.Errorf("Test %q unexpected error %s", name, err.Error())
		}

		if diff := cmp.Diff(scenario.expectedDefault, actualDefaultService); diff != "" {
			t.Errorf("Test %q unexpected default service (-want +got): %v", name, diff)
		}

		if scenario.inferenceService.Spec.Canary != nil {
			actualCanaryService, err := serviceBuilder.CreateTransformerService(
				constants.CanaryTransformerServiceName(isvc.Name),
				scenario.inferenceService.ObjectMeta,
				scenario.inferenceService.Spec.Canary.Transformer, true)
			if err != nil {
				t.Errorf("Test %q unexpected error %s", name, err.Error())
			}
			if diff := cmp.Diff(scenario.expectedCanary, actualCanaryService); diff != "" {
				t.Errorf("Test %q unexpected canary service (-want +got): %v", name, diff)
			}
		}

	}
}

func TestExplainerToKnativeService(t *testing.T) {
	isvc := v1alpha2.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mnist",
			Namespace: "default",
		},
		Spec: v1alpha2.InferenceServiceSpec{
			Default: v1alpha2.EndpointSpec{

				Predictor: v1alpha2.PredictorSpec{
					DeploymentSpec: v1alpha2.DeploymentSpec{
						MinReplicas:        1,
						MaxReplicas:        3,
						ServiceAccountName: "testsvcacc",
					},
					Tensorflow: &v1alpha2.TensorflowSpec{
						StorageURI:     "s3://test/mnist/export",
						RuntimeVersion: "1.13.0",
					},
				},
				Explainer: &v1alpha2.ExplainerSpec{
					Alibi: &v1alpha2.AlibiExplainerSpec{
						Type:           v1alpha2.AlibiAnchorsTabularExplainer,
						RuntimeVersion: "latest",
					},
				},
			},
		},
	}

	isvcCanary := isvc.DeepCopy()
	isvcCanary.Spec.CanaryTrafficPercent = 20
	isvcCanary.Spec.Canary = &v1alpha2.EndpointSpec{
		Predictor: v1alpha2.PredictorSpec{
			DeploymentSpec: v1alpha2.DeploymentSpec{
				MinReplicas:        1,
				MaxReplicas:        3,
				ServiceAccountName: "testsvcacc",
			},
			Tensorflow: &v1alpha2.TensorflowSpec{
				StorageURI:     "s3://test/mnist-2/export",
				RuntimeVersion: "1.13.0",
			},
		},
		Explainer: &v1alpha2.ExplainerSpec{
			Alibi: &v1alpha2.AlibiExplainerSpec{
				Type:           v1alpha2.AlibiAnchorsTabularExplainer,
				RuntimeVersion: "latest",
			},
		},
	}

	var defaultService = &knservingv1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.DefaultExplainerServiceName("mnist"),
			Namespace: "default",
		},
		Spec: knservingv1alpha1.ServiceSpec{
			ConfigurationSpec: knservingv1alpha1.ConfigurationSpec{
				Template: &knservingv1alpha1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"serving.kubeflow.org/inferenceservice": "mnist"},
						Annotations: map[string]string{
							"autoscaling.knative.dev/class":  "kpa.autoscaling.knative.dev",
							"autoscaling.knative.dev/target": "1",
						},
					},
					Spec: knservingv1alpha1.RevisionSpec{
						RevisionSpec: v1beta1.RevisionSpec{
							TimeoutSeconds: &constants.DefaultTimeout,
							PodSpec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Image: "alibi:latest",
										Args: []string{
											constants.ArgumentModelName,
											isvc.Name,
											constants.ArgumentPredictorHost,
											constants.DefaultPredictorServiceName(isvc.Name) + "." + isvc.Namespace,
											string(v1alpha2.AlibiAnchorsTabularExplainer),
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

	var canaryService = &knservingv1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.CanaryTransformerServiceName("mnist"),
			Namespace: "default",
		},
		Spec: knservingv1alpha1.ServiceSpec{
			ConfigurationSpec: knservingv1alpha1.ConfigurationSpec{
				Template: &knservingv1alpha1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"serving.kubeflow.org/inferenceservice": "mnist"},
						Annotations: map[string]string{
							"autoscaling.knative.dev/class":  "kpa.autoscaling.knative.dev",
							"autoscaling.knative.dev/target": "1",
						},
					},
					Spec: knservingv1alpha1.RevisionSpec{
						RevisionSpec: v1beta1.RevisionSpec{
							TimeoutSeconds: &constants.DefaultTimeout,
							PodSpec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Image: "alibi:latest",
										Args: []string{
											constants.ArgumentModelName,
											isvc.Name,
											constants.ArgumentPredictorHost,
											constants.CanaryPredictorServiceName(isvc.Name) + "." + isvc.Namespace,
											string(v1alpha2.AlibiAnchorsTabularExplainer),
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

	var configMapData = map[string]string{
		"explainers": `{
        "alibi" : {
            "image": "alibi"
        }
    }`,
	}
	scenarios := map[string]struct {
		inferenceService v1alpha2.InferenceService
		expectedDefault  *knservingv1alpha1.Service
		expectedCanary   *knservingv1alpha1.Service
	}{
		"RunLatestExplainer": {
			inferenceService: isvc,
			expectedDefault:  defaultService,
			expectedCanary:   nil,
		},
		"RunCanaryExplainer": {
			inferenceService: *isvcCanary,
			expectedDefault:  defaultService,
			expectedCanary:   canaryService,
		},
	}

	for name, scenario := range scenarios {
		serviceBuilder := NewServiceBuilder(c, &v1.ConfigMap{
			Data: configMapData,
		})
		actualDefaultService, err := serviceBuilder.CreateExplainerService(
			constants.DefaultExplainerServiceName(scenario.inferenceService.Name),
			scenario.inferenceService.ObjectMeta,
			scenario.inferenceService.Spec.Default.Explainer,
			constants.DefaultPredictorServiceName(scenario.inferenceService.Name)+"."+scenario.inferenceService.Namespace,
			false)
		if err != nil {
			t.Errorf("Test %q unexpected error %s", name, err.Error())
		}

		if diff := cmp.Diff(scenario.expectedDefault, actualDefaultService); diff != "" {
			t.Errorf("Test %q unexpected default service (-want +got): %v", name, diff)
		}

		if scenario.inferenceService.Spec.Canary != nil {
			actualCanaryService, err := serviceBuilder.CreateExplainerService(
				constants.CanaryTransformerServiceName(isvc.Name),
				scenario.inferenceService.ObjectMeta,
				scenario.inferenceService.Spec.Canary.Explainer,
				constants.CanaryPredictorServiceName(scenario.inferenceService.Name)+"."+scenario.inferenceService.Namespace,
				true)
			if err != nil {
				t.Errorf("Test %q unexpected error %s", name, err.Error())
			}
			if diff := cmp.Diff(scenario.expectedCanary, actualCanaryService); diff != "" {
				t.Errorf("Test %q unexpected canary service (-want +got): %v", name, diff)
			}
		}

	}
}
