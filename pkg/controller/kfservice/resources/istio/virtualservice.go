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

package istio

import (
	istionetworkingv1alpha3 "github.com/aspenmesh/istio-client-go/pkg/apis/networking/v1alpha3"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type VirtualServiceBuilder struct {
}

func NewVirtualServiceBuilder() *VirtualServiceBuilder {
	return &VirtualServiceBuilder{}
}

func (r *VirtualServiceBuilder) CreateVirtualService(kfsvc *v1alpha2.KFService) *istionetworkingv1alpha3.VirtualService {
	// TODO: actually populate spec!
	return &istionetworkingv1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:        constants.VirtualServiceName(kfsvc.Name),
			Namespace:   kfsvc.Namespace,
			Labels:      kfsvc.Labels,
			Annotations: kfsvc.Annotations,
		},
	}
}
