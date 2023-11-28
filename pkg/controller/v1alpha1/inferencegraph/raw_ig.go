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

var logger = logf.Log.WithName("InferenceGraphRawDeployer")

/*
This function helps to create core podspec for a given inference graph spec and router configuration
Also propagates headers onto podspec container environment variables.

This function makes sense to be used in raw k8s deployment mode
*/
func createInferenceGraphPodSpec(graph *v1alpha1api.InferenceGraph, config *RouterConfig) *v1.PodSpec {
	bytes, err := json.Marshal(graph.Spec)
	if err != nil {
		return nil
	}

	//Pod spec with 'router container with resource requirements' and 'affinity' as well
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

/*
A simple utility to create a basic meta object given name and namespace;  Can be extended to accept labels, annotations as well
*/
func constructGraphObjectMeta(name string, namespace string, annotations map[string]string,
	labels map[string]string) metav1.ObjectMeta {

	if annotations == nil {
		annotations = make(map[string]string)
	}

	if labels == nil {
		labels = make(map[string]string)
	}

	labels[constants.InferenceGraphLabel] = name

	objectMeta := metav1.ObjectMeta{
		Name:        name,
		Namespace:   namespace,
		Labels:      labels,
		Annotations: annotations,
	}

	return objectMeta
}

/*
Creates a component extension spec to pass through hpa annotation values such as minreplicas, maxreplicas, scalemetric etc.

ComponentExtensionSpec exists for the purpose of Inference service predict/transformer components.  But we are using here for
Inference graph raw deployment as well merely to pass through MaxReplicas, MinReplicas, ScaleTarget and ScaleMetric values
to reuse all the code writeen in v1beta1/inferenceservice/reconcilers.
*/
func constructGraphComponentExtensionSpec(annotations map[string]string) v1beta1.ComponentExtensionSpec {
	if annotations == nil {
		annotations = make(map[string]string)
	}

	var minReplicas, maxReplicas, target, metric string

	componentExtSpec := v1beta1.ComponentExtensionSpec{}

	// Inference graph can have these values defined in annotations section as shown below.
	//annotations:
	//	serving.kserve.io/class: "hpa"
	//	serving.kserve.io/max-scale: "7"
	//	serving.kserve.io/metric: rps
	//	serving.kserve.io/min-scale: "1"
	//	serving.kserve.io/target: "40"

	if scaling, ok := annotations[constants.AutoscalerClass]; ok && constants.AutoscalerClassType(scaling) == constants.AutoscalerClassHPA {

		// if min-scale annotation exists
		if minReplicas, ok = annotations[constants.InferenceGraphMinScaleAnnotationKey]; ok {
			if value, err := strconv.Atoi(minReplicas); err == nil {
				componentExtSpec.MinReplicas = &value
			}
		}

		// if max-scale annotation exists
		if maxReplicas, ok = annotations[constants.InferenceGraphMaxScaleAnnotationKey]; ok {
			if value, err := strconv.Atoi(maxReplicas); err == nil {
				componentExtSpec.MaxReplicas = value
			}
		}
		// if target annotation exists
		if target, ok = annotations[constants.InferenceGraphTargetAnnotationKey]; ok {
			if value, err := strconv.Atoi(target); err == nil {
				componentExtSpec.ScaleTarget = &value
			}
		}
		// if metric annotation exists
		if metric, ok = annotations[constants.InferenceGraphMetricsAnnotationKey]; ok {
			scaleMetric := v1beta1.ScaleMetric(metric)
			componentExtSpec.ScaleMetric = &scaleMetric
		}
	}

	return componentExtSpec
}

/*
Handles bulk of raw deployment logic for Inference graph controller
1. Constructs PodSpec
2. Constructs Meta and Extensionspec
3. Creates a reconciler
4. Set controller referneces
5. Finally reconcile
*/
func handleInferenceGraphRawDeployment(cl client.Client, scheme *runtime.Scheme, graph *v1alpha1api.InferenceGraph, routerConfig *RouterConfig) (ctrl.Result, error) {
	// create desired service object.
	desiredSvc := createInferenceGraphPodSpec(graph, routerConfig)

	objectMeta := constructGraphObjectMeta(graph.ObjectMeta.Name, graph.ObjectMeta.Namespace, graph.ObjectMeta.Annotations, graph.ObjectMeta.Labels)

	//componentExtensionSpec := constructGraphComponentExtensionSpec(graph.ObjectMeta.Annotations)

	//create the reconciler
	reconciler, err := raw.NewRawKubeReconciler(cl, scheme, objectMeta, nil, desiredSvc)

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
	logger.Info("reconciled:")

	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "fails to reconcile inference graph raw")
	}

	//TODO still some work todo here
	graph.Status.PropagateRawStatus(deployment, reconciler.URL)
	logger.Info("status propagated:")

	return ctrl.Result{}, nil
}
