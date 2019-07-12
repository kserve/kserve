/*
Copyright 2018 The Knative Authors.

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

package revision

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/knative/pkg/kmp"
	"github.com/knative/pkg/logging"
	"github.com/knative/pkg/logging/logkey"
	kpav1alpha1 "github.com/knative/serving/pkg/apis/autoscaling/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/revision/config"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/revision/resources"
	resourcenames "github.com/knative/serving/pkg/reconciler/v1alpha1/revision/resources/names"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	serviceTimeoutDuration = 5 * time.Minute
)

func (c *Reconciler) reconcileDeployment(ctx context.Context, rev *v1alpha1.Revision) error {
	ns := rev.Namespace
	deploymentName := resourcenames.Deployment(rev)
	logger := logging.FromContext(ctx).With(zap.String(logkey.Deployment, deploymentName))

	deployment, err := c.deploymentLister.Deployments(ns).Get(deploymentName)
	if apierrs.IsNotFound(err) {
		// Deployment does not exist. Create it.
		rev.Status.MarkDeploying("Deploying")
		deployment, err = c.createDeployment(ctx, rev)
		if err != nil {
			logger.Errorf("Error creating deployment %q: %v", deploymentName, err)
			return err
		}
		logger.Infof("Created deployment %q", deploymentName)
	} else if err != nil {
		logger.Errorf("Error reconciling deployment %q: %v", deploymentName, err)
		return err
	} else if !metav1.IsControlledBy(deployment, rev) {
		// Surface an error in the revision's status, and return an error.
		rev.Status.MarkResourceNotOwned("Deployment", deploymentName)
		return fmt.Errorf("Revision: %q does not own Deployment: %q", rev.Name, deploymentName)
	} else {
		// The deployment exists, but make sure that it has the shape that we expect.
		deployment, err = c.checkAndUpdateDeployment(ctx, rev, deployment)
		if err != nil {
			logger.Errorf("Error updating deployment %q: %v", deploymentName, err)
			return err
		}
	}

	// If a container keeps crashing (no active pods in the deployment although we want some)
	if *deployment.Spec.Replicas > 0 && deployment.Status.AvailableReplicas == 0 {
		pods, err := c.KubeClientSet.CoreV1().Pods(ns).List(metav1.ListOptions{LabelSelector: metav1.FormatLabelSelector(deployment.Spec.Selector)})
		if err != nil {
			logger.Errorf("Error getting pods: %v", err)
		} else if len(pods.Items) > 0 {
			// Arbitrarily grab the very first pod, as they all should be crashing
			pod := pods.Items[0]

			for _, status := range pod.Status.ContainerStatuses {
				if status.Name == resources.UserContainerName {
					if t := status.LastTerminationState.Terminated; t != nil {
						logger.Infof("%s marking exiting with: %d/%s", rev.Name, t.ExitCode, t.Message)
						rev.Status.MarkContainerExiting(t.ExitCode, t.Message)
					}
					break
				}
			}
		}
	}

	// Now that we have a Deployment, determine whether there is any relevant
	// status to surface in the Revision.
	if hasDeploymentTimedOut(deployment) && !rev.Status.IsActivationRequired() {
		rev.Status.MarkProgressDeadlineExceeded(fmt.Sprintf(
			"Unable to create pods for more than %d seconds.", resources.ProgressDeadlineSeconds))
		c.Recorder.Eventf(rev, corev1.EventTypeNormal, "ProgressDeadlineExceeded",
			"Revision %s not ready due to Deployment timeout", rev.Name)
	}

	return nil
}

func (c *Reconciler) reconcileImageCache(ctx context.Context, rev *v1alpha1.Revision) error {
	logger := logging.FromContext(ctx)

	ns := rev.Namespace
	imageName := resourcenames.ImageCache(rev)
	_, getImageCacheErr := c.imageLister.Images(ns).Get(imageName)
	if apierrs.IsNotFound(getImageCacheErr) {
		_, err := c.createImageCache(ctx, rev)
		if err != nil {
			logger.Errorf("Error creating image cache %q: %v", imageName, err)
			return err
		}
		logger.Infof("Created image cache %q", imageName)
	} else if getImageCacheErr != nil {
		logger.Errorf("Error reconciling image cache %q: %v", imageName, getImageCacheErr)
		return getImageCacheErr
	}

	return nil
}

func (c *Reconciler) reconcileKPA(ctx context.Context, rev *v1alpha1.Revision) error {
	ns := rev.Namespace
	kpaName := resourcenames.KPA(rev)
	logger := logging.FromContext(ctx)
	logger.Info("Reconciling KPA:", kpaName)

	kpa, err := c.podAutoscalerLister.PodAutoscalers(ns).Get(kpaName)
	if apierrs.IsNotFound(err) {
		// KPA does not exist. Create it.
		kpa, err = c.createKPA(ctx, rev)
		if err != nil {
			logger.Errorf("Error creating KPA %s: %v", kpaName, err)
			return err
		}
		logger.Info("Created KPA:", kpaName)
	} else if err != nil {
		logger.Errorf("Error reconciling kpa %s: %v", kpaName, err)
		return err
	} else if !metav1.IsControlledBy(kpa, rev) {
		// Surface an error in the revision's status, and return an error.
		rev.Status.MarkResourceNotOwned("PodAutoscaler", kpaName)
		return fmt.Errorf("Revision: %q does not own PodAutoscaler: %q", rev.Name, kpaName)
	}

	// Perhaps tha KPA spec changed underneath ourselves?
	// TODO(vagababov): required for #1997. Should be removed in 0.7,
	// to fix the protocol type when it's unset.
	tmpl := resources.MakeKPA(rev)
	if !equality.Semantic.DeepEqual(tmpl.Spec, kpa.Spec) {
		logger.Infof("KPA %s needs reconciliation", kpa.Name)

		want := kpa.DeepCopy()
		want.Spec = tmpl.Spec
		if kpa, err = c.ServingClientSet.AutoscalingV1alpha1().PodAutoscalers(kpa.Namespace).Update(want); err != nil {
			return err
		}
		// This change will trigger KPA -> SKS -> K8s service change;
		// and those after reconciliation will back progpagate here.
		rev.Status.MarkDeploying("Updating")
	}

	// Propagate the service name from the PA.
	rev.Status.ServiceName = kpa.Status.ServiceName

	// Reflect the KPA status in our own.
	cond := kpa.Status.GetCondition(kpav1alpha1.PodAutoscalerConditionReady)
	switch {
	case cond == nil:
		rev.Status.MarkActivating("Deploying", "")
		// If not ready => SKS did not report a service name, we can reliably use.
	case cond.Status == corev1.ConditionUnknown:
		rev.Status.MarkActivating(cond.Reason, cond.Message)
	case cond.Status == corev1.ConditionFalse:
		rev.Status.MarkInactive(cond.Reason, cond.Message)
	case cond.Status == corev1.ConditionTrue:
		rev.Status.MarkActive()

		// Precondition for PA being active is SKS being active and
		// that entices that |service.endpoints| > 0.
		rev.Status.MarkResourcesAvailable()
		rev.Status.MarkContainerHealthy()
	}
	return nil
}

func (c *Reconciler) reconcileFluentdConfigMap(ctx context.Context, rev *v1alpha1.Revision) error {
	logger := logging.FromContext(ctx)
	cfgs := config.FromContext(ctx)

	if !cfgs.Observability.EnableVarLogCollection {
		return nil
	}

	ns := rev.Namespace
	name := resourcenames.FluentdConfigMap(rev)

	configMap, err := c.configMapLister.ConfigMaps(ns).Get(name)
	if apierrs.IsNotFound(err) {
		// ConfigMap doesn't exist, going to create it
		desiredConfigMap := resources.MakeFluentdConfigMap(rev, cfgs.Observability)
		_, err = c.KubeClientSet.CoreV1().ConfigMaps(ns).Create(desiredConfigMap)
		if err != nil {
			logger.Errorw("Error creating fluentd configmap", zap.Error(err))
			return err
		}
		logger.Infof("Created fluentd configmap: %q", name)
	} else if err != nil {
		logger.Errorf("configmaps.Get for %q failed: %s", name, err)
		return err
	} else {
		desiredConfigMap := resources.MakeFluentdConfigMap(rev, cfgs.Observability)
		if !equality.Semantic.DeepEqual(configMap.Data, desiredConfigMap.Data) {
			diff, err := kmp.SafeDiff(desiredConfigMap.Data, configMap.Data)
			if err != nil {
				return fmt.Errorf("failed to diff ConfigMap: %v", err)
			}
			logger.Infof("Reconciling fluentd configmap diff (-desired, +observed): %v", diff)

			// Don't modify the informers copy
			existing := configMap.DeepCopy()
			existing.Data = desiredConfigMap.Data
			_, err = c.KubeClientSet.CoreV1().ConfigMaps(ns).Update(existing)
			if err != nil {
				logger.Errorw("Error updating fluentd configmap", zap.Error(err))
				return err
			}
		}
	}
	return nil
}
