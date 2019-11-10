package pod

import (
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"k8s.io/apimachinery/pkg/api/resource"
	"knative.dev/pkg/kmp"
	"testing"

	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	LoggerDefaultCPURequest    = "100m"
	LoggerDefaultCPULimit      = "1"
	LoggerDefaultMemoryRequest = "200Mi"
	LoggerDefaultMemoryLimit   = "1Gi"
)

var (
	loggerConfig = &LoggerConfig{
		Image:         "gcr.io/kfserving/logger:latest",
		CpuRequest:    LoggerDefaultCPURequest,
		CpuLimit:      LoggerDefaultCPULimit,
		MemoryRequest: LoggerDefaultMemoryRequest,
		MemoryLimit:   LoggerDefaultMemoryLimit,
	}

	loggerResourceRequirement = v1.ResourceRequirements{
		Limits: map[v1.ResourceName]resource.Quantity{
			v1.ResourceCPU:    resource.MustParse(LoggerDefaultCPULimit),
			v1.ResourceMemory: resource.MustParse(LoggerDefaultMemoryLimit),
		},
		Requests: map[v1.ResourceName]resource.Quantity{
			v1.ResourceCPU:    resource.MustParse(LoggerDefaultCPURequest),
			v1.ResourceMemory: resource.MustParse(LoggerDefaultMemoryRequest),
		},
	}
)

func TestLoggerInjector(t *testing.T) {
	scenarios := map[string]struct {
		original *v1.Pod
		expected *v1.Pod
	}{
		"AddLogger": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
					Annotations: map[string]string{
						constants.LoggerInternalAnnotationKey:        "true",
						constants.LoggerSinkUrlInternalAnnotationKey: "http://httpbin.org/",
						constants.LoggerModeInternalAnnotationKey:    string(v1alpha2.LogAll),
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					}},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
					Annotations: map[string]string{
						constants.LoggerInternalAnnotationKey:        "true",
						constants.LoggerSinkUrlInternalAnnotationKey: "http://httpbin.org/",
						constants.LoggerModeInternalAnnotationKey:    string(v1alpha2.LogAll),
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					},
						{
							Name:  LoggerContainerName,
							Image: loggerConfig.Image,
							Args: []string{
								"--log-url",
								"http://httpbin.org/",
								"--source-uri",
								"deployment",
								"--log-mode",
								"all",
								"--model-id",
								"",
							},
							Resources: loggerResourceRequirement,
						},
					},
				},
			},
		},
		"DoNotAddLogger": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					}},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					}},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		injector := &LoggerInjector{
			loggerConfig,
		}
		injector.InjectLogger(scenario.original)
		if diff, _ := kmp.SafeDiff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}
