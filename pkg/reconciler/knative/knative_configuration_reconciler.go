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
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("Reconciler")

type ConfigurationReconciler struct {
	client client.Client
}

func NewConfigurationReconciler(client client.Client) *ConfigurationReconciler {
	return &ConfigurationReconciler{
		client: client,
	}
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Service resource
// with the current status of the resource.
func (c *ConfigurationReconciler) Reconcile(ctx context.Context, desiredConfiguration *knservingv1alpha1.Configuration) (*knservingv1alpha1.Configuration, error) {
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

	if serviceSemanticEquals(desiredConfiguration, configuration) {
		// No differences to reconcile.
		return configuration, nil
	}

	diff, err := kmp.SafeDiff(desiredConfiguration.Spec, configuration.Spec)
	if err != nil {
		return configuration, fmt.Errorf("failed to diff configuration: %v", err)
	}
	log.Info("Reconciling configuration diff (-desired, +observed):", "diff", diff)

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

func serviceSemanticEquals(desiredConfiguration, configuration *knservingv1alpha1.Configuration) bool {
	return equality.Semantic.DeepEqual(desiredConfiguration.Spec, configuration.Spec) &&
		equality.Semantic.DeepEqual(desiredConfiguration.ObjectMeta.Labels, configuration.ObjectMeta.Labels) &&
		equality.Semantic.DeepEqual(desiredConfiguration.ObjectMeta.Annotations, configuration.ObjectMeta.Annotations)
}
