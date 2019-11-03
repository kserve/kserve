package pod

import (
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
							Image: LoggerContainerImage + ":" + LoggerContainerImageVersion,
							Args: []string{
								"--log-url",
								"http://httpbin.org/",
								"--source-uri",
								"deployment",
								"--log-mode",
								"all",
								"--model-id",
								"",
							}},
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
		injector := &LoggerInjector{}
		injector.InjectLogger(scenario.original)
		// cmd.Diff complains on ResourceList when Nvidia is key. Objects are explicitly compared
		if diff := cmp.Diff(
			scenario.expected.Spec,
			scenario.original.Spec,
		); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}

	}
}
