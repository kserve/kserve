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

	"github.com/google/go-cmp/cmp"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/constants"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	knativeserving "knative.dev/serving/pkg/apis/serving/v1beta1"
)

var kfsvc = v1alpha2.KFService{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "mnist",
		Namespace: "default",
		Annotations: map[string]string{
			constants.KFServiceGKEAcceleratorAnnotationKey: "nvidia-tesla-t4",
		},
	},
	Spec: v1alpha2.KFServiceSpec{
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

var defaultService = &knativeserving.Service{
	ObjectMeta: metav1.ObjectMeta{
		Name:      constants.DefaultPredictorServiceName("mnist"),
		Namespace: "default",
	},
	Spec: knativeserving.ServiceSpec{
		ConfigurationSpec: knativeserving.ConfigurationSpec{
			Template: knativeserving.RevisionTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"serving.kubeflow.org/kfservice": "mnist"},
					Annotations: map[string]string{
						"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
						"autoscaling.knative.dev/target":                           "1",
						"autoscaling.knative.dev/minScale":                         "1",
						"autoscaling.knative.dev/maxScale":                         "3",
						constants.KFServiceGKEAcceleratorAnnotationKey:             "nvidia-tesla-t4",
						constants.StorageInitializerSourceUriInternalAnnotationKey: kfsvc.Spec.Default.Predictor.Tensorflow.StorageURI,
					},
				},
				Spec: knativeserving.RevisionSpec{
					TimeoutSeconds: &constants.DefaultTimeout,
					PodSpec: v1.PodSpec{
						ServiceAccountName: "testsvcacc",
						Containers: []v1.Container{
							{
								Image:   v1alpha2.TensorflowServingImageName + ":" + kfsvc.Spec.Default.Predictor.Tensorflow.RuntimeVersion,
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
}

var canaryService = &knativeserving.Service{
	ObjectMeta: metav1.ObjectMeta{
		Name:      constants.CanaryPredictorServiceName("mnist"),
		Namespace: "default",
	},
	Spec: knativeserving.ServiceSpec{
		ConfigurationSpec: knativeserving.ConfigurationSpec{
			Template: knativeserving.RevisionTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"serving.kubeflow.org/kfservice": "mnist"},
					Annotations: map[string]string{
						"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
						"autoscaling.knative.dev/target":                           "1",
						"autoscaling.knative.dev/minScale":                         "1",
						"autoscaling.knative.dev/maxScale":                         "3",
						constants.KFServiceGKEAcceleratorAnnotationKey:             "nvidia-tesla-t4",
						constants.StorageInitializerSourceUriInternalAnnotationKey: "s3://test/mnist-2/export",
					},
				},
				Spec: knativeserving.RevisionSpec{
					TimeoutSeconds: &constants.DefaultTimeout,
					PodSpec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Image:   v1alpha2.TensorflowServingImageName + ":" + kfsvc.Spec.Default.Predictor.Tensorflow.RuntimeVersion,
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
}

func TestKFServiceToKnativeService(t *testing.T) {
	scenarios := map[string]struct {
		configMapData   map[string]string
		kfService       v1alpha2.KFService
		expectedDefault *knativeserving.Service
		expectedCanary  *knativeserving.Service
	}{
		"RunLatestModel": {
			kfService:       kfsvc,
			expectedDefault: defaultService,
			expectedCanary:  nil,
		},
		"RunCanaryModel": {
			kfService: v1alpha2.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
					Annotations: map[string]string{
						constants.KFServiceGKEAcceleratorAnnotationKey: "nvidia-tesla-t4",
					},
				},
				Spec: v1alpha2.KFServiceSpec{
					Default: v1alpha2.EndpointSpec{
						Predictor: v1alpha2.PredictorSpec{
							DeploymentSpec: v1alpha2.DeploymentSpec{
								MinReplicas:        1,
								MaxReplicas:        3,
								ServiceAccountName: "testsvcacc",
							},
							Tensorflow: &v1alpha2.TensorflowSpec{
								StorageURI:     kfsvc.Spec.Default.Predictor.Tensorflow.StorageURI,
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
				Status: v1alpha2.KFServiceStatus{
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
			kfService: v1alpha2.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sklearn",
					Namespace: "default",
				},
				Spec: v1alpha2.KFServiceSpec{
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
			expectedDefault: &knativeserving.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultPredictorServiceName("sklearn"),
					Namespace: "default",
				},
				Spec: knativeserving.ServiceSpec{
					ConfigurationSpec: knativeserving.ConfigurationSpec{
						Template: knativeserving.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kubeflow.org/kfservice": "sklearn"},
								Annotations: map[string]string{
									constants.StorageInitializerSourceUriInternalAnnotationKey: "s3://test/sklearn/export",
									"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/target":                           "1",
								},
							},
							Spec: knativeserving.RevisionSpec{
								TimeoutSeconds: &constants.DefaultTimeout,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: v1alpha2.SKLearnServerImageName + ":" + v1alpha2.DefaultSKLearnRuntimeVersion,
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
			kfService: v1alpha2.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "xgboost",
					Namespace: "default",
				},
				Spec: v1alpha2.KFServiceSpec{
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
			expectedDefault: &knativeserving.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultPredictorServiceName("xgboost"),
					Namespace: "default",
				},
				Spec: knativeserving.ServiceSpec{
					ConfigurationSpec: knativeserving.ConfigurationSpec{
						Template: knativeserving.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kubeflow.org/kfservice": "xgboost"},
								Annotations: map[string]string{
									constants.StorageInitializerSourceUriInternalAnnotationKey: "s3://test/xgboost/export",
									"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/target":                           "1",
								},
							},
							Spec: knativeserving.RevisionSpec{
								TimeoutSeconds: &constants.DefaultTimeout,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: v1alpha2.XGBoostServerImageName + ":" + v1alpha2.DefaultXGBoostRuntimeVersion,
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
			kfService: v1alpha2.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "xgboost",
					Namespace: "default",
				},
				Spec: v1alpha2.KFServiceSpec{
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
			expectedDefault: &knativeserving.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultPredictorServiceName("xgboost"),
					Namespace: "default",
				},
				Spec: knativeserving.ServiceSpec{
					ConfigurationSpec: knativeserving.ConfigurationSpec{
						Template: knativeserving.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kubeflow.org/kfservice": "xgboost"},
								Annotations: map[string]string{
									constants.StorageInitializerSourceUriInternalAnnotationKey: "s3://test/xgboost/export",
									"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/target":                           "1",
								},
							},
							Spec: knativeserving.RevisionSpec{
								TimeoutSeconds: &constants.DefaultTimeout,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: "kfserving/xgbserver:" + v1alpha2.DefaultXGBoostRuntimeVersion,
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
			kfService: v1alpha2.KFService{
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
				Spec: v1alpha2.KFServiceSpec{
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
			expectedDefault: &knativeserving.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultPredictorServiceName("sklearn"),
					Namespace: "default",
				},
				Spec: knativeserving.ServiceSpec{
					ConfigurationSpec: knativeserving.ConfigurationSpec{
						Template: knativeserving.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kubeflow.org/kfservice": "sklearn"},
								Annotations: map[string]string{
									constants.StorageInitializerSourceUriInternalAnnotationKey: "s3://test/sklearn/export",
									"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/target":                           "2",
									"autoscaling.knative.dev/minScale":                         "1",
									"sourceName":                                               "srcName",
									"prop1":                                                    "val1",
								},
							},
							Spec: knativeserving.RevisionSpec{
								TimeoutSeconds: &constants.DefaultTimeout,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: v1alpha2.SKLearnServerImageName + ":" + v1alpha2.DefaultSKLearnRuntimeVersion,
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
		serviceBuilder := NewServiceBuilder(c, &v1.ConfigMap{
			Data: scenario.configMapData,
		})
		actualDefaultService, err := serviceBuilder.CreatePredictorService(
			constants.DefaultPredictorServiceName(scenario.kfService.Name),
			scenario.kfService.ObjectMeta,
			&scenario.kfService.Spec.Default.Predictor,
		)
		if err != nil {
			t.Errorf("Test %q unexpected error %s", name, err.Error())
		}

		if diff := cmp.Diff(scenario.expectedDefault, actualDefaultService); diff != "" {
			t.Errorf("Test %q unexpected default service (-want +got): %v", name, diff)
		}

		if scenario.kfService.Spec.Canary != nil {
			actualCanaryService, err := serviceBuilder.CreatePredictorService(
				constants.CanaryPredictorServiceName(kfsvc.Name),
				scenario.kfService.ObjectMeta,
				&scenario.kfService.Spec.Canary.Predictor,
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
	kfsvc := v1alpha2.KFService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mnist",
			Namespace: "default",
		},
		Spec: v1alpha2.KFServiceSpec{
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

	kfsvcCanary := kfsvc.DeepCopy()
	kfsvcCanary.Spec.CanaryTrafficPercent = 20
	kfsvcCanary.Spec.Canary = &v1alpha2.EndpointSpec{
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

	var defaultService = &knativeserving.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.DefaultTransformerServiceName("mnist"),
			Namespace: "default",
		},
		Spec: knativeserving.ServiceSpec{
			ConfigurationSpec: knativeserving.ConfigurationSpec{
				Template: knativeserving.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"serving.kubeflow.org/kfservice": "mnist"},
						Annotations: map[string]string{
							"autoscaling.knative.dev/class":    "kpa.autoscaling.knative.dev",
							"autoscaling.knative.dev/target":   "1",
							"autoscaling.knative.dev/minScale": "1",
							"autoscaling.knative.dev/maxScale": "3",
						},
					},
					Spec: knativeserving.RevisionSpec{
						TimeoutSeconds: &constants.DefaultTimeout,
						PodSpec: v1.PodSpec{
							ServiceAccountName: "testsvcacc",
							Containers: []v1.Container{
								{
									Image: "transformer:latest",
									Args: []string{
										constants.ArgumentModelName,
										kfsvc.Name,
										constants.ArgumentPredictorHost,
										constants.DefaultPredictorServiceName(kfsvc.Name) + "." + kfsvc.Namespace,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	var canaryService = &knativeserving.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.CanaryTransformerServiceName("mnist"),
			Namespace: "default",
		},
		Spec: knativeserving.ServiceSpec{
			ConfigurationSpec: knativeserving.ConfigurationSpec{
				Template: knativeserving.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"serving.kubeflow.org/kfservice": "mnist"},
						Annotations: map[string]string{
							"autoscaling.knative.dev/class":    "kpa.autoscaling.knative.dev",
							"autoscaling.knative.dev/target":   "1",
							"autoscaling.knative.dev/minScale": "2",
							"autoscaling.knative.dev/maxScale": "4",
						},
					},
					Spec: knativeserving.RevisionSpec{
						TimeoutSeconds: &constants.DefaultTimeout,
						PodSpec: v1.PodSpec{
							ServiceAccountName: "testsvcacc",
							Containers: []v1.Container{
								{
									Image: "transformer:v2",
									Args: []string{
										constants.ArgumentModelName,
										kfsvc.Name,
										constants.ArgumentPredictorHost,
										constants.CanaryPredictorServiceName(kfsvc.Name) + "." + kfsvc.Namespace,
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
		configMapData   map[string]string
		kfService       v1alpha2.KFService
		expectedDefault *knativeserving.Service
		expectedCanary  *knativeserving.Service
	}{
		"RunLatestModel": {
			kfService:       kfsvc,
			expectedDefault: defaultService,
			expectedCanary:  nil,
		},
		"RunCanaryModel": {
			kfService:       *kfsvcCanary,
			expectedDefault: defaultService,
			expectedCanary:  canaryService,
		},
	}

	for name, scenario := range scenarios {
		serviceBuilder := NewServiceBuilder(c, &v1.ConfigMap{
			Data: scenario.configMapData,
		})
		actualDefaultService, err := serviceBuilder.CreateTransformerService(
			constants.DefaultTransformerServiceName(scenario.kfService.Name),
			scenario.kfService.ObjectMeta,
			scenario.kfService.Spec.Default.Transformer, false)
		if err != nil {
			t.Errorf("Test %q unexpected error %s", name, err.Error())
		}

		if diff := cmp.Diff(scenario.expectedDefault, actualDefaultService); diff != "" {
			t.Errorf("Test %q unexpected default service (-want +got): %v", name, diff)
		}

		if scenario.kfService.Spec.Canary != nil {
			actualCanaryService, err := serviceBuilder.CreateTransformerService(
				constants.CanaryTransformerServiceName(kfsvc.Name),
				scenario.kfService.ObjectMeta,
				scenario.kfService.Spec.Canary.Transformer, true)
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
	kfsvc := v1alpha2.KFService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mnist",
			Namespace: "default",
		},
		Spec: v1alpha2.KFServiceSpec{
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

	kfsvcCanary := kfsvc.DeepCopy()
	kfsvcCanary.Spec.CanaryTrafficPercent = 20
	kfsvcCanary.Spec.Canary = &v1alpha2.EndpointSpec{
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

	var defaultService = &knativeserving.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.DefaultExplainerServiceName("mnist"),
			Namespace: "default",
		},
		Spec: knativeserving.ServiceSpec{
			ConfigurationSpec: knativeserving.ConfigurationSpec{
				Template: knativeserving.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"serving.kubeflow.org/kfservice": "mnist"},
						Annotations: map[string]string{
							"autoscaling.knative.dev/class":  "kpa.autoscaling.knative.dev",
							"autoscaling.knative.dev/target": "1",
						},
					},
					Spec: knativeserving.RevisionSpec{
						TimeoutSeconds: &constants.DefaultTimeout,
						PodSpec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "alibi:latest",
									Args: []string{
										constants.ArgumentModelName,
										kfsvc.Name,
										constants.ArgumentPredictorHost,
										constants.DefaultPredictorServiceName(kfsvc.Name) + "." + kfsvc.Namespace,
										string(v1alpha2.AlibiAnchorsTabularExplainer),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	var canaryService = &knativeserving.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.CanaryTransformerServiceName("mnist"),
			Namespace: "default",
		},
		Spec: knativeserving.ServiceSpec{
			ConfigurationSpec: knativeserving.ConfigurationSpec{
				Template: knativeserving.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"serving.kubeflow.org/kfservice": "mnist"},
						Annotations: map[string]string{
							"autoscaling.knative.dev/class":  "kpa.autoscaling.knative.dev",
							"autoscaling.knative.dev/target": "1",
						},
					},
					Spec: knativeserving.RevisionSpec{
						TimeoutSeconds: &constants.DefaultTimeout,
						PodSpec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "alibi:latest",
									Args: []string{
										constants.ArgumentModelName,
										kfsvc.Name,
										constants.ArgumentPredictorHost,
										constants.CanaryPredictorServiceName(kfsvc.Name) + "." + kfsvc.Namespace,
										string(v1alpha2.AlibiAnchorsTabularExplainer),
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
		configMapData   map[string]string
		kfService       v1alpha2.KFService
		expectedDefault *knativeserving.Service
		expectedCanary  *knativeserving.Service
	}{
		"RunLatestExplainer": {
			kfService:       kfsvc,
			expectedDefault: defaultService,
			expectedCanary:  nil,
			configMapData:   configMapData,
		},
		"RunCanaryExplainer": {
			kfService:       *kfsvcCanary,
			expectedDefault: defaultService,
			expectedCanary:  canaryService,
			configMapData:   configMapData,
		},
	}

	for name, scenario := range scenarios {
		serviceBuilder := NewServiceBuilder(c, &v1.ConfigMap{
			Data: scenario.configMapData,
		})
		actualDefaultService, err := serviceBuilder.CreateExplainerService(
			constants.DefaultExplainerServiceName(scenario.kfService.Name),
			scenario.kfService.ObjectMeta,
			scenario.kfService.Spec.Default.Explainer,
			constants.DefaultPredictorServiceName(scenario.kfService.Name)+"."+scenario.kfService.Namespace,
			false)
		if err != nil {
			t.Errorf("Test %q unexpected error %s", name, err.Error())
		}

		if diff := cmp.Diff(scenario.expectedDefault, actualDefaultService); diff != "" {
			t.Errorf("Test %q unexpected default service (-want +got): %v", name, diff)
		}

		if scenario.kfService.Spec.Canary != nil {
			actualCanaryService, err := serviceBuilder.CreateExplainerService(
				constants.CanaryTransformerServiceName(kfsvc.Name),
				scenario.kfService.ObjectMeta,
				scenario.kfService.Spec.Canary.Explainer,
				constants.CanaryPredictorServiceName(scenario.kfService.Name)+"."+scenario.kfService.Namespace,
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
