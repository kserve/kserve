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

package raw

import (
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	deployment "github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/reconcilers/deployment"
	service "github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/reconcilers/service"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	knapis "knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//RawKubeReconciler reconciles the Native K8S Resources
type RawKubeReconciler struct {
	client     client.Client
	scheme     *runtime.Scheme
	Deployment *deployment.DeploymentReconciler
	Service    *service.ServiceReconciler
	URL        *knapis.URL
}

// RawKubeReconciler creates raw kubernetes resource reconciler.
func NewRawKubeReconciler(client client.Client,
	scheme *runtime.Scheme,
	componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec) *RawKubeReconciler {
	return &RawKubeReconciler{
		client:     client,
		scheme:     scheme,
		Deployment: deployment.NewDeploymentReconciler(client, scheme, componentMeta, componentExt, podSpec),
		Service:    service.NewServiceReconciler(client, scheme, componentMeta, componentExt, podSpec),
		URL:        createRawURL(client, componentMeta),
	}
}

func createRawURL(client client.Client, metadata metav1.ObjectMeta) *knapis.URL {
	ingressConfig, err := v1beta1.NewIngressConfig(client)
	if err != nil {
		if ingressConfig == nil {
			ingressConfig = &v1beta1.IngressConfig{}
		}
	}
	if ingressConfig.IngressDomain == "" {
		ingressConfig.IngressDomain = "example.com"
	}

	url := &knapis.URL{}
	url.Scheme = "http"
	url.Host = metadata.Name + "-" + metadata.Namespace + "." + ingressConfig.IngressDomain
	return url
}

//Reconcile ...
func (r *RawKubeReconciler) Reconcile() (*appsv1.Deployment, error) {
	//reconcile Deployment
	deployment, err := r.Deployment.Reconcile()
	if err != nil {
		return nil, err
	}
	//reconcile Service
	_, err = r.Service.Reconcile()
	if err != nil {
		return nil, err
	}
	//@TODO reconcile HPA
	return deployment, nil
}
