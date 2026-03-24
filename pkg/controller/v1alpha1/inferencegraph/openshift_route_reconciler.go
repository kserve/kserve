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
	"context"
	"reflect"

	v1 "github.com/openshift/api/route/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/pkg/network"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

type OpenShiftRouteReconciler struct {
	Scheme *runtime.Scheme
	Client client.Client
}

func (r *OpenShiftRouteReconciler) Reconcile(ctx context.Context, inferenceGraph *v1alpha1.InferenceGraph) (string, error) {
	logger := ctrlLog.FromContext(ctx, "subreconciler", "OpenShiftRoute")

	desiredRoute, err := r.buildOpenShiftRoute(inferenceGraph)
	if err != nil {
		return "", err
	}

	nsName := types.NamespacedName{
		Namespace: desiredRoute.Namespace,
		Name:      desiredRoute.Name,
	}

	actualRoute := v1.Route{}
	err = client.IgnoreNotFound(r.Client.Get(ctx, nsName, &actualRoute))
	if err != nil {
		return "", err
	}

	if val, ok := inferenceGraph.Labels[constants.NetworkVisibility]; ok && val == constants.ClusterLocalVisibility {
		privateHost := network.GetServiceHostname(inferenceGraph.GetName(), inferenceGraph.GetNamespace())
		// The IG is private. Remove the route, if needed.
		if len(actualRoute.Name) != 0 {
			logger.Info("Deleting OpenShift Route for InferenceGraph", "namespace", desiredRoute.Namespace, "name", desiredRoute.Name)
			err = r.Client.Delete(ctx, &actualRoute)
			if err != nil {
				return privateHost, err
			}
		}

		// Return private hostname.
		return privateHost, nil
	}

	if len(actualRoute.Name) == 0 {
		logger.Info("Creating a new OpenShift Route for InferenceGraph", "namespace", desiredRoute.Namespace, "name", desiredRoute.Name)
		err = r.Client.Create(ctx, &desiredRoute)
		return getRouteHostname(&desiredRoute), err
	}

	if !reflect.DeepEqual(actualRoute.Spec, desiredRoute.Spec) {
		logger.Info("Updating OpenShift Route for InferenceGraph", "namespace", desiredRoute.Namespace, "name", desiredRoute.Name)
		actualRoute.Spec = desiredRoute.Spec
		err = r.Client.Update(ctx, &actualRoute)
	}

	return getRouteHostname(&actualRoute), err
}

func (r *OpenShiftRouteReconciler) buildOpenShiftRoute(inferenceGraph *v1alpha1.InferenceGraph) (v1.Route, error) {
	route := v1.Route{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      inferenceGraph.Name + "-route",
			Namespace: inferenceGraph.Namespace,
		},
		Spec: v1.RouteSpec{
			To: v1.RouteTargetReference{
				Kind: "Service",
				Name: inferenceGraph.GetName(),
			},
			Port: &v1.RoutePort{
				TargetPort: intstr.FromString(inferenceGraph.GetName()),
			},
			TLS: &v1.TLSConfig{
				Termination: v1.TLSTerminationReencrypt,
			},
		},
	}

	err := controllerutil.SetControllerReference(inferenceGraph, &route, r.Scheme)
	return route, err
}

func getRouteHostname(route *v1.Route) string {
	for _, entry := range route.Status.Ingress {
		return entry.Host
	}
	return ""
}
