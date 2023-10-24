package inferencegraph

import (
	"encoding/json"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/raw"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strconv"

	v1alpha1api "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
)

var logger = logf.Log.WithName("Inference Graph RAWDeploymentMode")

func createInferenceGraphPodSpec(graph *v1alpha1api.InferenceGraph, config *RouterConfig) *v1.PodSpec {
	bytes, err := json.Marshal(graph.Spec)
	if err != nil {
		return nil
	}

	podSpec := &v1.PodSpec{
		Containers: []v1.Container{
			{
				Name:  graph.ObjectMeta.Name,
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

	// User can pass down scaling class annotation
	if scaling, ok := annotations[constants.AutoscalerClass]; ok && constants.AutoscalerClassType(scaling) == constants.AutoscalerClassHPA {
		if minReplicas, ok = annotations[constants.InferenceGraphMinScaleAnnotationKey]; ok {
			if value, err := strconv.Atoi(minReplicas); err == nil {
				componentExtSpec.MinReplicas = &value
			}
		}
		if maxReplicas, ok = annotations[constants.InferenceGraphMaxScaleAnnotationKey]; ok {
			if value, err := strconv.Atoi(maxReplicas); err == nil {
				componentExtSpec.MaxReplicas = value
			}
		}
		if target, ok = annotations[constants.InferenceGraphTargetAnnotationKey]; ok {
			if value, err := strconv.Atoi(target); err == nil {
				componentExtSpec.ScaleTarget = &value
			}
		}
		if metric, ok = annotations[constants.InferenceGraphMetricsAnnotationKey]; ok {
			scaleMetric := v1beta1.ScaleMetric(metric)
			componentExtSpec.ScaleMetric = &scaleMetric
		}
	}

	return componentExtSpec
}

func handleInferenceGraphRawDeployment(cl client.Client, scheme *runtime.Scheme, graph *v1alpha1api.InferenceGraph, routerConfig *RouterConfig) (ctrl.Result, error) {
	// create desired service object.
	desiredSvc := createInferenceGraphPodSpec(graph, routerConfig)
	log.Info("desired spec:", "desiredspec", desiredSvc)

	objectMeta := constructGraphObjectMeta("bmopuriigjenkinstest1", "kserve-test")

	componentExtensionSpec := constructGraphComponentExtensionSpec(graph.ObjectMeta.Annotations)
	log.Info("objectmeta:", "object meta", objectMeta)

	log.Info("extensionspec:", "extension spec", componentExtensionSpec)

	//create the reconciler
	reconciler, err := raw.NewRawKubeReconciler(cl, scheme, objectMeta, &componentExtensionSpec,
		desiredSvc)

	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "fails to create NewRawKubeReconciler for inference graph")
	}
	//set Deployment Controller
	if err := controllerutil.SetControllerReference(graph, reconciler.Deployment.Deployment, scheme); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "fails to set deployment owner reference for inference graph")
	}
	//set Service Controller
	if err := controllerutil.SetControllerReference(graph, reconciler.Service.Service, scheme); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "fails to set service owner reference for inference graph")
	}

	//set autoscaler Controller
	if err := reconciler.Scaler.Autoscaler.SetControllerReferences(graph, scheme); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "fails to set autoscaler owner references for inference graph")
	}

	//reconcile
	deployment, err := reconciler.Reconcile()
	log.Info("reconciled:")

	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "fails to reconcile inference graph raw")
	}

	graph.Status.PropagateRawStatus(deployment, reconciler.URL)
	log.Info("status propagated:")

	return ctrl.Result{}, nil
}
