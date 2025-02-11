package inferencegraph

import (
	"context"
	"fmt"
	"reflect"

	v1 "github.com/openshift/api/route/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
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
			Name:      fmt.Sprintf("%s-route", inferenceGraph.Name),
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
