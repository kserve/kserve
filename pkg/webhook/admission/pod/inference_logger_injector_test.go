package pod

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestInferenceLoggerInjector(t *testing.T) {
	scenarios := map[string]struct {
		original *v1.Pod
		expected *v1.Pod
	}{
		"AddInferenceLogger": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
					Annotations: map[string]string{
						constants.InferenceLoggerSinkUrlInternalAnnotationKey: "http://httpbin.org/",
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
						constants.InferenceLoggerSinkUrlInternalAnnotationKey: "http://httpbin.org/",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					},
						{
							Name:  InferenceLoggerContainerName,
							Image: InferenceLoggerContainerImage + ":" + InferenceLoggerContainerImageVersion,
							Args: []string{
								"--log_url",
								"http://httpbin.org/",
							},},
					},
				},
			},
		},
		"DoNotAddInferenceLogger": {
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
		injector := &InferenceLoggerInjector{}
		injector.InjectInferenceLogger(scenario.original)
		// cmd.Diff complains on ResourceList when Nvidia is key. Objects are explicitly compared
		if diff := cmp.Diff(
			scenario.expected.Spec,
			scenario.original.Spec,
		); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}

	}
}