package knservice

import (
	"context"
	"github.com/google/go-cmp/cmp"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/containers/tensorflow"
	"github.com/onsi/gomega"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestKnativeServiceReconcile(t *testing.T) {
	existingService := &knservingv1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mnist",
			Namespace: "default",
		},
		Spec: knservingv1alpha1.ServiceSpec{
			Release: &knservingv1alpha1.ReleaseType{
				Revisions: []string{"@latest"},
				Configuration: knservingv1alpha1.ConfigurationSpec{
					RevisionTemplate: knservingv1alpha1.RevisionTemplateSpec{
						Spec: knservingv1alpha1.RevisionSpec{
							Container: v1.Container{
								Image:   "tensorflow/serving:1.13",
								Command: []string{tensorflow.TensorflowEntrypointCommand},
								Args: []string{
									"--port=" + tensorflow.TensorflowServingPort,
									"--rest_api_port=" + tensorflow.TensorflowServingRestPort,
									"--model_name=mnist",
									"--model_base_path=s3://test/mnist/export",
								},
							},
						},
					},
				},
			},
		},
	}
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		desiredService *knservingv1alpha1.Service
		update         bool
		shouldFail     bool
	}{
		"Reconcile new model serving ": {
			update: false,
			desiredService: &knservingv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ServiceSpec{
					Release: &knservingv1alpha1.ReleaseType{
						Revisions: []string{"@latest"},
						Configuration: knservingv1alpha1.ConfigurationSpec{
							RevisionTemplate: knservingv1alpha1.RevisionTemplateSpec{
								Spec: knservingv1alpha1.RevisionSpec{
									Container: v1.Container{
										Image:   "tensorflow/serving:1.13",
										Command: []string{tensorflow.TensorflowEntrypointCommand},
										Args: []string{
											"--port=" + tensorflow.TensorflowServingPort,
											"--rest_api_port=" + tensorflow.TensorflowServingRestPort,
											"--model_name=mnist",
											"--model_base_path=s3://test/mnist/export",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"Reconcile model path update": {
			update: true,
			desiredService: &knservingv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ServiceSpec{
					Release: &knservingv1alpha1.ReleaseType{
						Revisions: []string{"@latest"},
						Configuration: knservingv1alpha1.ConfigurationSpec{
							RevisionTemplate: knservingv1alpha1.RevisionTemplateSpec{
								Spec: knservingv1alpha1.RevisionSpec{
									Container: v1.Container{
										Image:   "tensorflow/serving:1.13",
										Command: []string{tensorflow.TensorflowEntrypointCommand},
										Args: []string{
											"--port=" + tensorflow.TensorflowServingPort,
											"--rest_api_port=" + tensorflow.TensorflowServingRestPort,
											"--model_name=mnist",
											"--model_base_path=s3://test/mnist-v2/export",
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

	serviceReconciler := NewServiceReconcile(c)
	for name, scenario := range scenarios {
		if scenario.update {
			g.Expect(c.Create(context.TODO(), existingService)).NotTo(gomega.HaveOccurred())
		}
		service, err := serviceReconciler.Reconcile(context.TODO(), scenario.desiredService)
		// Validate
		if scenario.shouldFail && err == nil {
			t.Errorf("Test %q failed: returned success but expected error", name)
		}
		if !scenario.shouldFail {
			if err != nil {
				t.Errorf("Test %q failed: returned error: %v", name, err)
			}
			if diff := cmp.Diff(scenario.desiredService.Spec, service.Spec); diff != "" {
				t.Errorf("Test %q unexpected ServiceSpec (-want +got): %v", name, diff)
			}
		}
		g.Expect(c.Delete(context.TODO(), existingService)).NotTo(gomega.HaveOccurred())
	}
}
