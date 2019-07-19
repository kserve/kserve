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
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/kfservice/resources/knative"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("Reconciler")

type ConfigurationReconciler struct {
	client               client.Client
	scheme               *runtime.Scheme
	configurationBuilder *knative.ConfigurationBuilder
}

func NewConfigurationReconciler(client client.Client, scheme *runtime.Scheme, config *v1.ConfigMap) *ConfigurationReconciler {
	return &ConfigurationReconciler{
		client:               client,
		scheme:               scheme,
		configurationBuilder: knative.NewConfigurationBuilder(client, config),
	}
}

func (r *ConfigurationReconciler) Reconcile(kfsvc *v1alpha1.KFService) error {
	if err := r.reconcileDefault(kfsvc); err != nil {
		return err
	}
	if err := r.reconcileCanary(kfsvc); err != nil {
		return err
	}
	return nil
}

func (r *ConfigurationReconciler) reconcileDefault(kfsvc *v1alpha1.KFService) error {
	defaultConfiguration, err := r.configurationBuilder.CreateKnativeConfiguration(
		constants.DefaultConfigurationName(kfsvc.Name),
		kfsvc.ObjectMeta,
		&kfsvc.Spec.Default,
	)
	if err != nil {
		return err
	}

	status, err := r.reconcileConfiguration(kfsvc, defaultConfiguration)
	if err != nil {
		return err
	}

	kfsvc.Status.PropagateDefaultConfigurationStatus(status)
	return nil
}

func (r *ConfigurationReconciler) reconcileCanary(kfsvc *v1alpha1.KFService) error {
	canaryConfigurationName := constants.CanaryConfigurationName(kfsvc.Name)
	if kfsvc.Spec.Canary == nil {
		if kfsvc.Status.Canary.Name != "" {
			existing := &knservingv1alpha1.Configuration{}
			err := r.client.Get(context.TODO(), types.NamespacedName{Name: canaryConfigurationName, Namespace: kfsvc.Namespace}, existing)
			if err != nil {
				if !errors.IsNotFound(err) {
					return err
				}
			} else {
				log.Info("Deleting Knative Serving configuration", "namespace", kfsvc.Namespace, "name", canaryConfigurationName)
				err := r.client.Delete(context.TODO(), existing, client.PropagationPolicy(metav1.DeletePropagationForeground))
				if err != nil {
					if !errors.IsNotFound(err) {
						return err
					}
				}
			}
			kfsvc.Status.ResetCanaryConfigurationStatus()
		}
		return nil
	}

	canaryConfiguration, err := r.configurationBuilder.CreateKnativeConfiguration(
		canaryConfigurationName,
		kfsvc.ObjectMeta,
		kfsvc.Spec.Canary,
	)
	if err != nil {
		return err
	}

	status, err := r.reconcileConfiguration(kfsvc, canaryConfiguration)
	if err != nil {
		return err
	}

	kfsvc.Status.PropagateCanaryConfigurationStatus(status)
	return nil
}

func (r *ConfigurationReconciler) reconcileConfiguration(kfsvc *v1alpha1.KFService, desired *knservingv1alpha1.Configuration) (*knservingv1alpha1.ConfigurationStatus, error) {
	if err := controllerutil.SetControllerReference(kfsvc, desired, r.scheme); err != nil {
		return nil, err
	}
	// Create configuration if does not exist
	existing := &knservingv1alpha1.Configuration{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating Knative Serving configuration", "namespace", desired.Namespace, "name", desired.Name)
			return &desired.Status, r.client.Create(context.TODO(), desired)
		}
		return nil, err
	}

	// Return if no differences to reconcile.
	if semanticEquals(desired, existing) {
		return &existing.Status, nil
	}

	// Reconcile differences and update
	diff, err := kmp.SafeDiff(desired.Spec, existing.Spec)
	if err != nil {
		return &existing.Status, fmt.Errorf("failed to diff configuration: %v", err)
	}
	log.Info("Reconciling configuration diff (-desired, +observed):", "diff", diff)
	log.Info("Updating configuration", "namespace", desired.Namespace, "name", desired.Name)
	existing.Spec = desired.Spec
	existing.ObjectMeta.Labels = desired.ObjectMeta.Labels
	existing.ObjectMeta.Annotations = desired.ObjectMeta.Annotations
	if err := r.client.Update(context.TODO(), existing); err != nil {
		return &existing.Status, err
	}

	return &existing.Status, nil
}

func semanticEquals(desiredConfiguration, configuration *knservingv1alpha1.Configuration) bool {
	return equality.Semantic.DeepEqual(desiredConfiguration.Spec, configuration.Spec) &&
		equality.Semantic.DeepEqual(desiredConfiguration.ObjectMeta.Labels, configuration.ObjectMeta.Labels) &&
		equality.Semantic.DeepEqual(desiredConfiguration.ObjectMeta.Annotations, configuration.ObjectMeta.Annotations)
}
