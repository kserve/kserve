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
	"github.com/kubeflow/kfserving/pkg/credentials"
	"github.com/kubeflow/kfserving/pkg/reconciler/knative/resources"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
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
	configurationBuilder resources.ConfigurationBuilder
	credentialBuilder    credentials.CredentialBuilder
}

func NewConfigurationReconciler(client client.Client, scheme *runtime.Scheme, config *v1.ConfigMap) *ConfigurationReconciler {
	return &ConfigurationReconciler{
		client:               client,
		scheme:               scheme,
		configurationBuilder: resources.NewConfigurationBuilder(client, config),
	}
}

func (r *ConfigurationReconciler) Reconcile(kfsvc *v1alpha1.KFService) error {
	// Create Default
	defaultConfiguration, err := r.configurationBuilder.CreateKnativeConfiguration(
		constants.DefaultConfigurationName(kfsvc.Name),
		kfsvc.ObjectMeta,
		&kfsvc.Spec.Default,
	)
	if err != nil {
		return err
	}

	if err := controllerutil.SetControllerReference(kfsvc, defaultConfiguration, r.scheme); err != nil {
		return err
	}

	if err := r.reconcileConfiguration(kfsvc, defaultConfiguration); err != nil {
		return err
	}

	kfsvc.Status.PropagateDefaultConfigurationStatus(&defaultConfiguration.Status)

	if kfsvc.Spec.Canary == nil {
		return nil
	}

	// Create Canary
	canaryConfiguration, err := r.configurationBuilder.CreateKnativeConfiguration(
		constants.CanaryConfigurationName(kfsvc.Name),
		kfsvc.ObjectMeta,
		kfsvc.Spec.Canary,
	)
	if err != nil {
		return err
	}

	if err := controllerutil.SetControllerReference(kfsvc, canaryConfiguration, r.scheme); err != nil {
		return err
	}

	if err := r.reconcileConfiguration(kfsvc, canaryConfiguration); err != nil {
		return err
	}

	kfsvc.Status.PropagateCanaryConfigurationStatus(&canaryConfiguration.Status)
	return nil
}

func (r *ConfigurationReconciler) reconcileConfiguration(kfsvc *v1alpha1.KFService, desired *knservingv1alpha1.Configuration) error {
	// Create configuration if does not exist
	existing := &knservingv1alpha1.Configuration{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating Knative Serving configuration", "namespace", desired.Namespace, "name", desired.Name)
			return r.client.Create(context.TODO(), desired)
		}
		return err
	}

	// Return if no differences to reconcile.
	if semanticEquals(desired, existing) {
		return nil
	}

	// Reconcile differences and update
	diff, err := kmp.SafeDiff(desired.Spec, existing.Spec)
	if err != nil {
		return fmt.Errorf("failed to diff configuration: %v", err)
	}
	log.Info("Reconciling configuration diff (-desired, +observed):", "diff", diff)
	log.Info("Updating configuration", "namespace", desired.Namespace, "name", desired.Name)
	existing.Spec = desired.Spec
	existing.ObjectMeta.Labels = desired.ObjectMeta.Labels
	existing.ObjectMeta.Annotations = desired.ObjectMeta.Annotations
	if err := r.client.Update(context.TODO(), existing); err != nil {
		return err
	}

	return nil
}

func semanticEquals(desiredConfiguration, configuration *knservingv1alpha1.Configuration) bool {
	return equality.Semantic.DeepEqual(desiredConfiguration.Spec, configuration.Spec) &&
		equality.Semantic.DeepEqual(desiredConfiguration.ObjectMeta.Labels, configuration.ObjectMeta.Labels) &&
		equality.Semantic.DeepEqual(desiredConfiguration.ObjectMeta.Annotations, configuration.ObjectMeta.Annotations)
}
