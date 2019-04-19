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
func (c *ServiceReconciler) Reconcile(ctx context.Context, desiredService *knservingv1alpha1.Service) (*knservingv1alpha1.Service, error) {
	service := &knservingv1alpha1.Service{}
	err := c.client.Get(context.TODO(), types.NamespacedName{Name: desiredService.Name, Namespace: desiredService.Namespace}, service)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating Knative Serving Service", "namespace", service.Namespace, "name", service.Name)
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
	log.Info("Updating service", "namespace", service.Namespace, "name", service.Name)
	err = c.client.Update(context.TODO(), service)
	if err != nil {
		return service, err
	}
	return service, nil
}

func serviceSemanticEquals(desiredService, service *knservingv1alpha1.Service) bool {
	return equality.Semantic.DeepEqual(desiredService.Spec, service.Spec) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Labels, service.ObjectMeta.Labels) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Annotations, service.ObjectMeta.Annotations)
}
