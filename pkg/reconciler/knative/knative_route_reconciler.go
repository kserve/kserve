package knative

import (
	"context"
	"fmt"
	"github.com/knative/pkg/kmp"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RouteReconciler struct {
	client client.Client
}

func NewRouteReconciler(client client.Client) *RouteReconciler {
	return &RouteReconciler{
		client: client,
	}
}

func (c *RouteReconciler) Reconcile(ctx context.Context, desiredRoute *knservingv1alpha1.Route) (*knservingv1alpha1.Route, error) {
	route := &knservingv1alpha1.Route{}
	err := c.client.Get(context.TODO(), types.NamespacedName{Name: desiredRoute.Name, Namespace: desiredRoute.Namespace}, route)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating Knative Serving route", "namespace", desiredRoute.Namespace, "name", desiredRoute.Name)
			err = c.client.Create(context.TODO(), desiredRoute)
			return desiredRoute, err
		}
		return nil, err
	}

	if routeSemanticEquals(desiredRoute, route) {
		// No differences to reconcile.
		return route, nil
	}

	diff, err := kmp.SafeDiff(desiredRoute.Spec, route.Spec)
	if err != nil {
		return route, fmt.Errorf("failed to diff route: %v", err)
	}
	log.Info("Reconciling route diff (-desired, +observed):", "diff", diff)

	route.Spec = desiredRoute.Spec
	route.ObjectMeta.Labels = desiredRoute.ObjectMeta.Labels
	route.ObjectMeta.Annotations = desiredRoute.ObjectMeta.Annotations
	log.Info("Updating route", "namespace", route.Namespace, "name", route.Name)
	err = c.client.Update(context.TODO(), route)
	if err != nil {
		return route, err
	}
	return route, nil
}

func routeSemanticEquals(desiredRoute, route *knservingv1alpha1.Route) bool {
	return equality.Semantic.DeepEqual(desiredRoute.Spec, route.Spec) &&
		equality.Semantic.DeepEqual(desiredRoute.ObjectMeta.Labels, route.ObjectMeta.Labels) &&
		equality.Semantic.DeepEqual(desiredRoute.ObjectMeta.Annotations, route.ObjectMeta.Annotations)
}
