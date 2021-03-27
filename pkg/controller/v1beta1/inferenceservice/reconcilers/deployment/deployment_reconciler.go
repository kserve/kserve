/*
Copyright 2021 kubeflow.org.
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

package deployment

import (
	"context"

	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("DeploymentReconciler")

//DeploymentReconciler is the struct of Raw K8S Object
type DeploymentReconciler struct {
	client       client.Client
	scheme       *runtime.Scheme
	Deployment   *appsv1.Deployment
	componentExt *v1beta1.ComponentExtensionSpec
}

func NewDeploymentReconciler(client client.Client,
	scheme *runtime.Scheme,
	componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec) *DeploymentReconciler {
	return &DeploymentReconciler{
		client:       client,
		scheme:       scheme,
		Deployment:   createRawDeployment(componentMeta, componentExt, podSpec),
		componentExt: componentExt,
	}
}

func createRawDeployment(componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec) *appsv1.Deployment {
	var minReplicas int32
	if componentExt.MinReplicas == nil {
		minReplicas = int32(constants.DefaultMinReplicas)
	} else {
		minReplicas = int32(*componentExt.MinReplicas)
	}

	if minReplicas < int32(constants.DefaultMinReplicas) {
		minReplicas = int32(constants.DefaultMinReplicas)
	}
	podMetadata := componentMeta
	podMetadata.Labels["app"] = constants.GetRawServiceLabel(componentMeta.Name)
	setDefaultPodSpec(podSpec)
	deployment := &appsv1.Deployment{
		ObjectMeta: componentMeta,
		Spec: appsv1.DeploymentSpec{
			Replicas: &minReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": constants.GetRawServiceLabel(componentMeta.Name),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: podMetadata,
				Spec:       corev1.PodSpec(*podSpec),
			},
		},
	}
	setDefaultDeploymentSpec(&deployment.Spec)
	return deployment
}

//checkDeploymentExist checks if the deployment exists?
func (r *DeploymentReconciler) checkDeploymentExist(client client.Client) (constants.CheckResultType, *appsv1.Deployment, error) {
	//get deployment
	existingDeployment := &appsv1.Deployment{}
	err := client.Get(context.TODO(), types.NamespacedName{
		Namespace: r.Deployment.Namespace,
		Name:      r.Deployment.Name,
	}, existingDeployment)
	if err != nil {
		if apierr.IsNotFound(err) {
			return constants.CheckResultCreate, existingDeployment, nil
		}
		return constants.CheckResultUnknown, existingDeployment, err
	}
	//existed, check equivalent
	if semanticDeploymentEquals(r.Deployment, existingDeployment) {
		return constants.CheckResultExisted, existingDeployment, nil
	}
	return constants.CheckResultUpdate, existingDeployment, nil
}

func semanticDeploymentEquals(desired, existing *appsv1.Deployment) bool {
	return equality.Semantic.DeepEqual(desired.Spec, existing.Spec)
}

func setDefaultPodSpec(podSpec *corev1.PodSpec) {
	if podSpec.DNSPolicy == "" {
		podSpec.DNSPolicy = corev1.DNSClusterFirst
	}
	if podSpec.RestartPolicy == "" {
		podSpec.RestartPolicy = corev1.RestartPolicyAlways
	}
	if podSpec.TerminationGracePeriodSeconds == nil {
		TerminationGracePeriodSeconds := int64(corev1.DefaultTerminationGracePeriodSeconds)
		podSpec.TerminationGracePeriodSeconds = &TerminationGracePeriodSeconds
	}
	if podSpec.SecurityContext == nil {
		podSpec.SecurityContext = &corev1.PodSecurityContext{}
	}
	if podSpec.SchedulerName == "" {
		podSpec.SchedulerName = corev1.DefaultSchedulerName
	}
	for i := range podSpec.Containers {
		container := &podSpec.Containers[i]
		if container.TerminationMessagePath == "" {
			container.TerminationMessagePath = "/dev/termination-log"
		}
		if container.TerminationMessagePolicy == "" {
			container.TerminationMessagePolicy = corev1.TerminationMessageReadFile
		}
		if container.ImagePullPolicy == "" {
			container.ImagePullPolicy = corev1.PullIfNotPresent
		}
	}
}

func setDefaultDeploymentSpec(spec *appsv1.DeploymentSpec) {
	if spec.Strategy.Type == "" {
		spec.Strategy.Type = appsv1.RollingUpdateDeploymentStrategyType
	}
	if spec.Strategy.RollingUpdate == nil {
		spec.Strategy.RollingUpdate = &appsv1.RollingUpdateDeployment{
			MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
			MaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
		}
	}
	if spec.RevisionHistoryLimit == nil {
		revisionHistoryLimit := int32(10)
		spec.RevisionHistoryLimit = &revisionHistoryLimit
	}
	if spec.ProgressDeadlineSeconds == nil {
		progressDeadlineSeconds := int32(600)
		spec.ProgressDeadlineSeconds = &progressDeadlineSeconds
	}
}

//Reconcile ...
func (r *DeploymentReconciler) Reconcile() (*appsv1.Deployment, error) {
	//reconcile Deployment
	checkResult, deployment, err := r.checkDeploymentExist(r.client)
	if err != nil {
		return nil, err
	}
	log.Info("deployment reconcile", "checkResult", checkResult, "err", err)
	if checkResult == constants.CheckResultCreate {
		err = r.client.Create(context.TODO(), r.Deployment)
	} else if checkResult == constants.CheckResultUpdate { //CheckResultUpdate
		err = r.client.Update(context.TODO(), r.Deployment)
	}
	if err != nil {
		return nil, err
	}

	return deployment, nil
}
