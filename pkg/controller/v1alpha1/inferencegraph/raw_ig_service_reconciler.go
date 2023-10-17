package inferencegraph

import (
	"encoding/json"

	v1alpha1api "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"reflect"
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

func constructGraphResourceRequirements(graph v1alpha1api.InferenceGraph, config RouterConfig) v1.ResourceRequirements {
	var specResources v1.ResourceRequirements
	if !reflect.ValueOf(graph.Spec.Resources).IsZero() {
		logger.Info("Ignoring defaults for ResourceRequirements as spec has resources mentioned", "specResources", graph.Spec.Resources)
		specResources = v1.ResourceRequirements{
			Limits:   graph.Spec.Resources.Limits,
			Requests: graph.Spec.Resources.Requests,
		}
	} else {
		specResources = v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceCPU:    resource.MustParse(config.CpuLimit),
				v1.ResourceMemory: resource.MustParse(config.MemoryLimit),
			},
			Requests: v1.ResourceList{
				v1.ResourceCPU:    resource.MustParse(config.CpuRequest),
				v1.ResourceMemory: resource.MustParse(config.MemoryRequest),
			},
		}
	}
	return specResources
}
