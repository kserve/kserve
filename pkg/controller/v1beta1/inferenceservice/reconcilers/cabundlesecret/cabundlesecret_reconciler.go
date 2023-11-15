/*
Copyright 2021 The KServe Authors.

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

package cabundlesecret

import (
	"context"
	"encoding/json"
	"fmt"

	kservev1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/webhook/admission/pod"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/kmp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("CaBundleSecretReconciler")

type CaBundleSecretReconciler struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewCaBundleSecretReconciler(client client.Client, scheme *runtime.Scheme) *CaBundleSecretReconciler {
	return &CaBundleSecretReconciler{
		client: client,
		scheme: scheme,
	}
}

func (c *CaBundleSecretReconciler) Reconcile(isvc *kservev1beta1.InferenceService) error {
	log.Info("Reconciling CaBundleSecret")

	isvcConfigMap := &corev1.ConfigMap{}
	err := c.client.Get(context.TODO(), types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace}, isvcConfigMap)
	if err != nil {
		log.Error(err, "Failed to find config map", "name", constants.InferenceServiceConfigMapName)
		return err
	}

	storageInitializerConfig := &pod.StorageInitializerConfig{}
	if storageInitializerConfigValue, ok := isvcConfigMap.Data["storageInitializer"]; ok {
		err := json.Unmarshal([]byte(storageInitializerConfigValue), &storageInitializerConfig)
		if err != nil {
			panic(fmt.Errorf("Unable to unmarshall storage initializer json string due to %w ", err))
		}
	}

	var newCaBundleSecret *corev1.Secret
	if storageInitializerConfig.CaBundleSecretName == "" {
		return nil
	} else {
		newCaBundleSecret, err = c.getCabundleSecretForUserNS(storageInitializerConfig.CaBundleSecretName, constants.KServeNamespace, isvc.Namespace)
		if err != nil {
			return fmt.Errorf("fails to get cabundle secret for creating to user namespace: %w", err)
		}
	}

	if err := c.ReconcileCaBundleSecret(newCaBundleSecret); err != nil {
		return fmt.Errorf("fails to reconcile ca bundle secret: %w", err)
	}

	return nil
}

func (c *CaBundleSecretReconciler) getCabundleSecretForUserNS(caBundleNameInConfig string, kserveNamespace string, isvcNamespace string) (*corev1.Secret, error) {
	var newCaBundleSecret *corev1.Secret
	log.Info("In caBundleNameInConfig")

	// Check if cabundle Secret exist & the cabundle.crt exist in the data in controller namespace
	// If it does not exist, return error
	caBundleSecret := &corev1.Secret{}
	if err := c.client.Get(context.TODO(),
		types.NamespacedName{Name: caBundleNameInConfig, Namespace: kserveNamespace}, caBundleSecret); err == nil {

		if cabundleSecretData := caBundleSecret.Data[constants.DefaultCaBundleFileName]; cabundleSecretData == nil {
			return nil, fmt.Errorf("specified cabundle file %s not found in cabundle secret %s",
				constants.DefaultCaBundleFileName, caBundleNameInConfig)
		} else {
			secretData := map[string][]byte{
				constants.DefaultCaBundleFileName: cabundleSecretData,
			}
			newCaBundleSecret = getDesiredCaBundleSecretForUserNS(constants.DefaultGlobalCaBundleSecretName, isvcNamespace, secretData)
		}
	} else {
		return nil, fmt.Errorf("can't read cabundle secret %s: %w", constants.DefaultCaBundleFileName, err)
	}

	return newCaBundleSecret, nil
}

func getDesiredCaBundleSecretForUserNS(secretName string, namespace string, cabundleData map[string][]byte) *corev1.Secret {
	desiredSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: cabundleData,
	}

	return desiredSecret
}

// ReconcileCaBundleSecret will manage the creation, update and deletion of the ca bundle Secret
func (c *CaBundleSecretReconciler) ReconcileCaBundleSecret(desiredSecret *corev1.Secret) error {
	log.Info("Reconciling ReconcileCaBundleSecret")

	// Create secret if does not exist
	existingSecret := &corev1.Secret{}
	err := c.client.Get(context.TODO(), types.NamespacedName{Name: desiredSecret.Name, Namespace: desiredSecret.Namespace}, existingSecret)
	if err != nil {
		if apierr.IsNotFound(err) {
			log.Info("Creating cabundle secret", "namespace", desiredSecret.Namespace, "name", desiredSecret.Name)
			err = c.client.Create(context.TODO(), desiredSecret)
		}
		return err
	}

	// Return if no differences to reconcile.
	if equality.Semantic.DeepEqual(desiredSecret, existingSecret) {
		return nil
	}

	// Reconcile differences and update
	diff, err := kmp.SafeDiff(desiredSecret.Data, existingSecret.Data)
	if err != nil {
		return fmt.Errorf("failed to diff cabundle secret: %w", err)
	}
	log.Info("Reconciling cabundle secret diff (-desired, +observed):", "diff", diff)
	log.Info("Updating cabundle secret", "namespace", existingSecret.Namespace, "name", existingSecret.Name)
	existingSecret.Data = desiredSecret.Data
	err = c.client.Update(context.TODO(), existingSecret)
	if err != nil {
		return fmt.Errorf("fails to update cabundle secret: %w", err)
	}

	return nil
}
