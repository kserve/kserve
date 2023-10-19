package inferencegraph

import (
	"encoding/json"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/serving/pkg/apis/autoscaling"
	"strconv"

	v1alpha1api "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
)

var logger = logf.Log.WithName("GraphKsvcReconciler")

type GraphServiceReconciler struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewGraphServiceReconciler(client client.Client,
	scheme *runtime.Scheme,
) *GraphServiceReconciler {
	return &GraphServiceReconciler{
		client: client,
		scheme: scheme,
	}
}

func createGraphService(graph *v1alpha1api.InferenceGraph, config *RouterConfig) *v1.PodSpec {
	bytes, err := json.Marshal(graph.Spec)
	if err != nil {
		return nil
	}

	podSpec := &v1.PodSpec{
		Containers: []v1.Container{
			{
				Name:  "bmopuriigjenkinstest1",
				Image: config.Image,
				Args: []string{
					"--graph-json",
					string(bytes),
				},
				Resources: constructResourceRequirements(*graph, *config),
			},
		},
		Affinity: graph.Spec.Affinity,
	}

	// Only adding this env variable "PROPAGATE_HEADERS" if router's headers config has the key "propagate"
	value, exists := config.Headers["propagate"]
	if exists {
		podSpec.Containers[0].Env = []v1.EnvVar{
			{
				Name:  constants.RouterHeadersPropagateEnvVar,
				Value: strings.Join(value, ","),
			},
		}
	}

	return podSpec
}

func constructGraphObjectMeta(name string, namespace string) metav1.ObjectMeta {
	objectMeta := metav1.ObjectMeta{
		Name:        name,
		Namespace:   namespace,
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
	}

	return objectMeta
}

func constructGraphComponentExtensionSpec(annotations map[string]string) v1beta1.ComponentExtensionSpec {

	if annotations == nil {
		annotations = make(map[string]string)
	}

	var minReplicas, maxReplicas, target, metric string

	componentExtSpec := v1beta1.ComponentExtensionSpec{}

	// User can pass down scaling class annotation to overwrite the default scaling KPA
	if scaling, ok := annotations[autoscaling.ClassAnnotationKey]; ok && scaling == autoscaling.HPA {
		if minReplicas, ok = annotations[autoscaling.MinScaleAnnotationKey]; ok {
			if value, err := strconv.Atoi(minReplicas); err == nil {
				componentExtSpec.MinReplicas = &value
			}
		}
		if maxReplicas, ok = annotations[autoscaling.MaxScaleAnnotationKey]; ok {
			if value, err := strconv.Atoi(maxReplicas); err == nil {
				componentExtSpec.MaxReplicas = value
			}
		}
		if target, ok = annotations[autoscaling.TargetAnnotationKey]; ok {
			if value, err := strconv.Atoi(target); err == nil {
				componentExtSpec.ScaleTarget = &value
			}
		}
		if metric, ok = annotations[autoscaling.MetricAnnotationKey]; ok {
			scaleMetric := v1beta1.ScaleMetric(metric)
			componentExtSpec.ScaleMetric = &scaleMetric
		}
	}

	return componentExtSpec
}
