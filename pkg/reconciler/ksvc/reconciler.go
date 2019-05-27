/*
Copyright 2019 kubeflow.org.

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

package ksvc

import (
	"context"
	"fmt"
	"github.com/knative/pkg/kmp"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("ServiceReconciler")

type ServiceReconciler struct {
	client client.Client
}

func NewServiceReconciler(client client.Client) *ServiceReconciler {
	return &ServiceReconciler{
		client: client,
	}
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Service resource
// with the current status of the resource.
func (c *ServiceReconciler) ReconcileConfiguration(ctx context.Context, desiredConfiguration *knservingv1alpha1.Configuration) (*knservingv1alpha1.Configuration, error) {
	configuration := &knservingv1alpha1.Configuration{}
	err := c.client.Get(context.TODO(), types.NamespacedName{Name: desiredConfiguration.Name,
		Namespace: desiredConfiguration.Namespace}, configuration)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating Knative Serving configuration", "namespace",
				desiredConfiguration.Namespace, "name", desiredConfiguration.Name)
			err = c.client.Create(context.TODO(), desiredConfiguration)
			return desiredConfiguration, err
		}
		return nil, err
	}

	if configurationSemanticEquals(desiredConfiguration, configuration) {
		// No differences to reconcile.
		return configuration, nil
	}

	diff, err := kmp.SafeDiff(desiredConfiguration.Spec, configuration.Spec)
	if err != nil {
		return configuration, fmt.Errorf("failed to diff configuration: %v", err)
	}
	log.Info("Reconciling configuration diff (-desired, +observed): %s", "diff", diff)

	configuration.Spec = desiredConfiguration.Spec
	configuration.ObjectMeta.Labels = desiredConfiguration.ObjectMeta.Labels
	configuration.ObjectMeta.Annotations = desiredConfiguration.ObjectMeta.Annotations
	log.Info("Updating configuration", "namespace", configuration.Namespace, "name", configuration.Name)
	err = c.client.Update(context.TODO(), configuration)
	if err != nil {
		return configuration, err
	}
	return configuration, nil
}

func (c *ServiceReconciler) ReconcileRoute(ctx context.Context, desiredRoute *knservingv1alpha1.Route) (*knservingv1alpha1.Route, error) {
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
	log.Info("Reconciling route diff (-desired, +observed): %s", "diff", diff)

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

func (c *ServiceReconciler) ReconcileService(ctx context.Context, desiredService *knservingv1alpha1.Service) (*knservingv1alpha1.Service, error) {
	service := &knservingv1alpha1.Service{}
	err := c.client.Get(context.TODO(), types.NamespacedName{Name: desiredService.Name,
		Namespace: desiredService.Namespace}, service)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating Knative Serving service", "namespace",
				desiredService.Namespace, "name", desiredService.Name)
			err = c.client.Create(context.TODO(), desiredService)
			return desiredService, err
		}
		return nil, err
	}

	if serviceSemanticEquals(desiredService, service) {
		// No differences to reconcile.
		return service, nil
	}

	diff, err := kmp.SafeDiff(desiredService.Spec, service.Spec)
	if err != nil {
		return service, fmt.Errorf("failed to diff service: %v", err)
	}
	log.Info("Reconciling service diff (-desired, +observed): %s", "diff", diff)

	service.Spec = desiredService.Spec
	service.ObjectMeta.Labels = desiredService.ObjectMeta.Labels
	service.ObjectMeta.Annotations = desiredService.ObjectMeta.Annotations
	log.Info("Updating service", "namespace", service.Namespace, "name", service.Name)
	err = c.client.Update(context.TODO(), service)
	if err != nil {
		return service, err
	}
	return service, nil
}

func configurationSemanticEquals(desiredConfiguration, configuration *knservingv1alpha1.Configuration) bool {
	return equality.Semantic.DeepEqual(desiredConfiguration.Spec, configuration.Spec) &&
		equality.Semantic.DeepEqual(desiredConfiguration.ObjectMeta.Labels, configuration.ObjectMeta.Labels) &&
		equality.Semantic.DeepEqual(desiredConfiguration.ObjectMeta.Annotations, configuration.ObjectMeta.Annotations)
}

func routeSemanticEquals(desiredRoute, route *knservingv1alpha1.Route) bool {
	return equality.Semantic.DeepEqual(desiredRoute.Spec, route.Spec) &&
		equality.Semantic.DeepEqual(desiredRoute.ObjectMeta.Labels, route.ObjectMeta.Labels) &&
		equality.Semantic.DeepEqual(desiredRoute.ObjectMeta.Annotations, route.ObjectMeta.Annotations)
}

func serviceSemanticEquals(desiredService, service *knservingv1alpha1.Service) bool {
	return equality.Semantic.DeepEqual(desiredService.Spec, service.Spec) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Labels, service.ObjectMeta.Labels) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Annotations, service.ObjectMeta.Annotations)
}
