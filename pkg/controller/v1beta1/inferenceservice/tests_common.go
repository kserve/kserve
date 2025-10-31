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

package inferenceservice

import (
	"fmt"
	"maps"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

const (
	REPLICAS                    int32 = 1
	REVISION_HISTORY            int32 = 10
	PROGRESSION_DEADLINE_SECODS int32 = 600
	GRACE_PERIOD                int64 = 30

	fastTimeout = time.Second * 3
	timeout     = time.Second * 60
	interval    = time.Millisecond * 250
	domain      = "example.com"
)

var (
	defaultResource = corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}

	defaultSecurityContext = &corev1.PodSecurityContext{
		SELinuxOptions:      nil,
		WindowsOptions:      nil,
		RunAsUser:           nil,
		RunAsGroup:          nil,
		RunAsNonRoot:        nil,
		SupplementalGroups:  nil,
		FSGroup:             nil,
		Sysctls:             nil,
		FSGroupChangePolicy: nil,
		SeccompProfile:      nil,
	}

	// common storageUri used by the tests
	storageUri = "s3://test/mnist/export"

	createInferenceServiceConfigMap = func(cnf map[string]string) *corev1.ConfigMap {
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.InferenceServiceConfigMapName,
				Namespace: constants.KServeNamespace,
			},
			Data: maps.Clone(cnf),
		}
	}
)

func getServingRuntime(name string, namespace string) v1alpha1.ServingRuntime {
	return v1alpha1.ServingRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha1.ServingRuntimeSpec{
			SupportedModelFormats: []v1alpha1.SupportedModelFormat{
				{
					Name:       "tensorflow",
					Version:    ptr.To("1"),
					AutoSelect: ptr.To(true),
				},
			},
			ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
				Containers: []corev1.Container{
					{
						Name:    constants.InferenceServiceContainerName,
						Image:   "tensorflow/serving:1.14.0",
						Command: []string{"/usr/bin/tensorflow_model_server"},
						Args: []string{
							"--port=9000",
							"--rest_api_port=8080",
							"--model_base_path=/mnt/models",
							"--rest_api_timeout_in_ms=60000",
						},
						Resources: defaultResource,
					},
				},
			},
			Disabled: ptr.To(false),
		},
	}
}

func getExectedService(predictorServiceKey types.NamespacedName, serviceName string) corev1.Service {
	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      predictorServiceKey.Name,
			Namespace: predictorServiceKey.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       constants.PredictorServiceName(serviceName),
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.IntOrString{Type: 0, IntVal: 8080, StrVal: ""},
				},
			},
			Type:            "ClusterIP",
			SessionAffinity: "None",
			Selector: map[string]string{
				"app": "isvc." + constants.PredictorServiceName(serviceName),
			},
		},
	}
}

func getExpectedIsvcStatus(serviceKey types.NamespacedName) v1beta1.InferenceServiceStatus {
	return v1beta1.InferenceServiceStatus{
		Status: duckv1.Status{
			Conditions: duckv1.Conditions{
				{
					Type:   v1beta1.IngressReady,
					Status: "True",
				},
				{
					Type:   v1beta1.PredictorReady,
					Status: "True",
				},
				{
					Type:   apis.ConditionReady,
					Status: "True",
				},
				{
					Type:     v1beta1.Stopped,
					Status:   "False",
					Severity: apis.ConditionSeverityInfo,
				},
			},
		},
		URL: &apis.URL{
			Scheme: "http",
			Host:   "raw-foo-default.example.com",
		},
		Address: &duckv1.Addressable{
			URL: &apis.URL{
				Scheme: "http",
				Host:   fmt.Sprintf("%s-predictor.%s.svc.cluster.local", serviceKey.Name, serviceKey.Namespace),
			},
		},
		Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
			v1beta1.PredictorComponent: {
				LatestCreatedRevision: "",
				URL: &apis.URL{
					Scheme: "http",
					Host:   "raw-foo-predictor-default.example.com",
				},
			},
		},
		ModelStatus: v1beta1.ModelStatus{
			TransitionStatus:    "InProgress",
			ModelRevisionStates: &v1beta1.ModelRevisionStates{TargetModelState: "Pending"},
			ModelCopies:         &v1beta1.ModelCopies{},
		},
		DeploymentMode:     string(constants.Standard),
		ServingRuntimeName: "tf-serving-raw",
	}
}

// getCommonPredictorExtensionSpec returns a new TensorFlow serving spec instance
func getCommonPredictorExtensionSpec() v1beta1.PredictorExtensionSpec {
	return v1beta1.PredictorExtensionSpec{
		StorageURI:     &storageUri,
		RuntimeVersion: ptr.To("1.14.0"),
		Container: corev1.Container{
			Name:      constants.InferenceServiceContainerName,
			Resources: defaultResource,
		},
	}
}

// getDefaultAnnotations returns the default annotations used on most of the ISVCs
func getDefaultAnnotations(scalerClass constants.AutoscalerClassType) map[string]string {
	return map[string]string{
		constants.DeploymentMode:  string(constants.Standard),
		constants.AutoscalerClass: string(scalerClass),
	}
}

func getDefaultMetrics() []v1beta1.MetricsSpec {
	return []v1beta1.MetricsSpec{
		{
			Type: v1beta1.PodMetricSourceType,
			PodMetric: &v1beta1.PodMetricSource{
				Metric: v1beta1.PodMetrics{
					Backend:     v1beta1.OpenTelemetryBackend,
					MetricNames: []string{"process_cpu_seconds_total"},
					Query:       "avg(process_cpu_seconds_total)",
				},
				Target: v1beta1.MetricTarget{
					Type:  v1beta1.ValueMetricType,
					Value: v1beta1.NewMetricQuantity(""),
				},
			},
		},
	}
}

func getDefaultRollingStrategy() appsv1.DeploymentStrategy {
	return appsv1.DeploymentStrategy{
		Type: "RollingUpdate",
		RollingUpdate: &appsv1.RollingUpdateDeployment{
			MaxUnavailable: &intstr.IntOrString{Type: 1, IntVal: 0, StrVal: "25%"},
			MaxSurge:       &intstr.IntOrString{Type: 1, IntVal: 0, StrVal: "25%"},
		},
	}
}

func getExpectedDeployment(explainerDeploymentKey types.NamespacedName, serviceName string, serviceKey types.NamespacedName, predictorServiceKey types.NamespacedName) appsv1.Deployment {
	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      explainerDeploymentKey.Name,
			Namespace: explainerDeploymentKey.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(REPLICAS),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "isvc." + explainerDeploymentKey.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      explainerDeploymentKey.Name,
					Namespace: "default",
					Labels: map[string]string{
						"app":                                 "isvc." + explainerDeploymentKey.Name,
						constants.KServiceComponentLabel:      constants.Explainer.String(),
						constants.InferenceServicePodLabelKey: serviceName,
					},
					Annotations: getDefaultAnnotations(constants.AutoscalerClassHPA),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "kserve/art-explainer:latest",
							Name:  constants.InferenceServiceContainerName,
							Args: []string{
								"--model_name",
								serviceKey.Name,
								"--http_port",
								"8080",
								"--predictor_host",
								fmt.Sprintf("%s.%s", predictorServiceKey.Name, predictorServiceKey.Namespace),
								"--adversary_type",
								"SquareAttack",
								"--nb_classes",
								"10",
							},
							Resources: defaultResource,
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.IntOrString{
											IntVal: 8080,
										},
									},
								},
								InitialDelaySeconds: 0,
								TimeoutSeconds:      1,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: "File",
							ImagePullPolicy:          "IfNotPresent",
						},
					},
					SchedulerName:                 "default-scheduler",
					RestartPolicy:                 "Always",
					TerminationGracePeriodSeconds: ptr.To(GRACE_PERIOD),
					DNSPolicy:                     "ClusterFirst",
					SecurityContext:               defaultSecurityContext,
					AutomountServiceAccountToken:  ptr.To(false),
				},
			},
			Strategy:                getDefaultRollingStrategy(),
			RevisionHistoryLimit:    ptr.To(REVISION_HISTORY),
			ProgressDeadlineSeconds: ptr.To(PROGRESSION_DEADLINE_SECODS),
		},
	}
}

func getDeploymentWithKServiceLabel(predictorDeploymentKey types.NamespacedName, serviceName string, isvc *v1beta1.InferenceService) appsv1.Deployment {
	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      predictorDeploymentKey.Name,
			Namespace: predictorDeploymentKey.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(REPLICAS),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "isvc." + predictorDeploymentKey.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      predictorDeploymentKey.Name,
					Namespace: "default",
					Labels: map[string]string{
						"app":                                 "isvc." + predictorDeploymentKey.Name,
						constants.KServiceComponentLabel:      constants.Predictor.String(),
						constants.InferenceServicePodLabelKey: serviceName,
					},
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: *isvc.Spec.Predictor.Model.StorageURI,
						constants.DeploymentMode:                                   string(constants.Standard),
						constants.AutoscalerClass:                                  string(constants.AutoscalerClassHPA),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "tensorflow/serving:" +
								*isvc.Spec.Predictor.Model.RuntimeVersion,
							Name:    constants.InferenceServiceContainerName,
							Command: []string{v1beta1.TensorflowEntrypointCommand},
							Args: []string{
								"--port=" + v1beta1.TensorflowServingGRPCPort,
								"--rest_api_port=" + v1beta1.TensorflowServingRestPort,
								"--model_base_path=" + constants.DefaultModelLocalMountPath,
								"--rest_api_timeout_in_ms=60000",
							},
							Resources: defaultResource,
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.IntOrString{
											IntVal: 8080,
										},
									},
								},
								InitialDelaySeconds: 0,
								TimeoutSeconds:      1,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: "File",
							ImagePullPolicy:          "IfNotPresent",
						},
					},
					SchedulerName:                 "default-scheduler",
					RestartPolicy:                 "Always",
					TerminationGracePeriodSeconds: ptr.To(GRACE_PERIOD),
					DNSPolicy:                     "ClusterFirst",
					SecurityContext:               defaultSecurityContext,
					AutomountServiceAccountToken:  ptr.To(false),
				},
			},
			Strategy:                getDefaultRollingStrategy(),
			RevisionHistoryLimit:    ptr.To(REVISION_HISTORY),
			ProgressDeadlineSeconds: ptr.To(PROGRESSION_DEADLINE_SECODS),
		},
	}
}
