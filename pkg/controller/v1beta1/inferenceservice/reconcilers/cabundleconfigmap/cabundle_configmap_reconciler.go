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

package cabundleconfigmap

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/kmp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	kservev1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/webhook/admission/pod"
)

var log = logf.Log.WithName("CaBundleConfigMapReconciler")

type CaBundleConfigMapReconciler struct {
	client    client.Client
	clientset kubernetes.Interface
	scheme    *runtime.Scheme
}

func NewCaBundleConfigMapReconciler(client client.Client, clientset kubernetes.Interface, scheme *runtime.Scheme) *CaBundleConfigMapReconciler {
	return &CaBundleConfigMapReconciler{
		client:    client,
		clientset: clientset,
		scheme:    scheme,
	}
}

func (c *CaBundleConfigMapReconciler) Reconcile(isvc *kservev1beta1.InferenceService) error {
	log.Info("Reconciling CaBundleConfigMap", "namespace", isvc.Namespace)
	isvcConfigMap, err := c.clientset.CoreV1().ConfigMaps(constants.KServeNamespace).Get(context.TODO(), constants.InferenceServiceConfigMapName, metav1.GetOptions{})
	if err != nil {
		log.Error(err, "failed to find config map", "name", constants.InferenceServiceConfigMapName)
		return err
	}

	storageInitializerConfig := &pod.StorageInitializerConfig{}
	if storageInitializerConfigValue, ok := isvcConfigMap.Data["storageInitializer"]; ok {
		err := json.Unmarshal([]byte(storageInitializerConfigValue), &storageInitializerConfig)
		if err != nil {
			return fmt.Errorf("unable to unmarshal storage initializer json string due to %w ", err)
		}
	}

	var newCaBundleConfigMap *corev1.ConfigMap
	if storageInitializerConfig.CaBundleConfigMapName == "" {
		return nil
	} else {
		newCaBundleConfigMap, err = c.getCabundleConfigMapForUserNS(storageInitializerConfig.CaBundleConfigMapName, constants.KServeNamespace, isvc.Namespace)
		if err != nil {
			return fmt.Errorf("fails to get cabundle configmap for creating to user namespace: %w", err)
		}
	}

	if err := c.ReconcileCaBundleConfigMap(newCaBundleConfigMap); err != nil {
		return fmt.Errorf("fails to reconcile cabundle configmap: %w", err)
	}

	return nil
}

func (c *CaBundleConfigMapReconciler) getCabundleConfigMapForUserNS(caBundleNameInConfig string, kserveNamespace string, isvcNamespace string) (*corev1.ConfigMap, error) {
	var newCaBundleConfigMap *corev1.ConfigMap

	// Check if cabundle configmap exist & the cabundle.crt exist in the data in controller namespace
	// If it does not exist, return error
	caBundleConfigMap, err := c.clientset.CoreV1().ConfigMaps(kserveNamespace).Get(context.TODO(), caBundleNameInConfig, metav1.GetOptions{})

	if err == nil {
		if caBundleConfigMapData := caBundleConfigMap.Data[constants.DefaultCaBundleFileName]; caBundleConfigMapData == "" {
			return nil, fmt.Errorf("specified cabundle file %s not found in cabundle configmap %s",
				constants.DefaultCaBundleFileName, caBundleNameInConfig)
		} else {
			configData := map[string]string{
				constants.DefaultCaBundleFileName: caBundleConfigMapData,
			}
			newCaBundleConfigMap = getDesiredCaBundleConfigMapForUserNS(constants.DefaultGlobalCaBundleConfigMapName, isvcNamespace, configData)
		}
	} else {
		return nil, errors.Wrapf(err, "failed to get configmap %s from the cluster", caBundleNameInConfig)
	}

	return newCaBundleConfigMap, nil
}

func getDesiredCaBundleConfigMapForUserNS(configmapName string, namespace string, cabundleData map[string]string) *corev1.ConfigMap {
	desiredConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configmapName,
			Namespace: namespace,
		},
		Data: cabundleData,
	}

	return desiredConfigMap
}

// ReconcileCaBundleConfigMap will manage the creation, update and deletion of the ca bundle ConfigMap
func (c *CaBundleConfigMapReconciler) ReconcileCaBundleConfigMap(desiredConfigMap *corev1.ConfigMap) error {

	// Create ConfigMap if does not exist
	existingConfigMap, err := c.clientset.CoreV1().ConfigMaps(desiredConfigMap.Namespace).Get(context.TODO(), desiredConfigMap.Name, metav1.GetOptions{})
	if err != nil {
		if apierr.IsNotFound(err) {
			log.Info("Creating cabundle configmap", "namespace", desiredConfigMap.Namespace, "name", desiredConfigMap.Name)
			err = c.client.Create(context.TODO(), desiredConfigMap)
		}
		return err
	}

	// Return if no differences to reconcile.
	if equality.Semantic.DeepEqual(desiredConfigMap, existingConfigMap) {
		return nil
	}

	// Reconcile differences and update
	diff, err := kmp.SafeDiff(desiredConfigMap.Data, existingConfigMap.Data)
	if err != nil {
		return fmt.Errorf("failed to diff cabundle configmap: %w", err)
	}
	log.V(1).Info("Reconciling cabundle configmap diff (-desired, +observed):", "diff", diff)
	log.Info("Updating cabundle configmap", "namespace", existingConfigMap.Namespace, "name", existingConfigMap.Name)
	existingConfigMap.Data = desiredConfigMap.Data
	err = c.client.Update(context.TODO(), existingConfigMap)
	if err != nil {
		return fmt.Errorf("fails to update cabundle configmap: %w", err)
	}

	return nil
}
