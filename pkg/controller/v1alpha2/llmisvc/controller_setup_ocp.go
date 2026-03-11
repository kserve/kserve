//go:build distro

/*
Copyright 2025 The KServe Authors.

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

package llmisvc

import (
	"fmt"

	istioapi "istio.io/client-go/pkg/apis/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/kserve/kserve/pkg/utils"
)

//+kubebuilder:rbac:groups=networking.istio.io,resources=destinationrules,verbs=get;list;watch;create;update;patch;delete

func extendControllerSetup(mgr manager.Manager, b *builder.Builder) error {
	if err := istioapi.AddToScheme(mgr.GetScheme()); err != nil {
		return fmt.Errorf("failed to add Istio v1 APIs to scheme: %w", err)
	}
	if ok, err := utils.IsCrdAvailable(mgr.GetConfig(), istioapi.SchemeGroupVersion.String(), "DestinationRule"); ok && err == nil {
		b.Owns(&istioapi.DestinationRule{}, builder.WithPredicates(childResourcesPredicate))
	}
	return nil
}
