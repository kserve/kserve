// Copyright 2026 The KServe Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"context"
	"fmt"
	"strings"

	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("OrphanCleanup")

// DeleteOrphans deletes objects matching labels whose names are not in expectedNames.
// All matching objects are processed before any deletion errors are returned.
func DeleteOrphans(ctx context.Context, c client.Client, list client.ObjectList, namespace string, labels client.MatchingLabels, expectedNames map[string]bool) error {
	gvk, err := apiutil.GVKForObject(list, c.Scheme())
	if err != nil {
		return fmt.Errorf("fails to determine resource kind for orphan cleanup: %w", err)
	}
	resourceKind := strings.TrimSuffix(gvk.Kind, "List")

	if err := c.List(ctx, list, client.InNamespace(namespace), labels); err != nil {
		return fmt.Errorf("fails to list %s resources for cleanup: %w", resourceKind, err)
	}

	var errs []error
	if err := meta.EachListItem(list, func(item runtime.Object) error {
		obj, ok := item.(client.Object)
		if !ok {
			errs = append(errs, fmt.Errorf("fails to clean up %s resources: list item %T is not a client object", resourceKind, item))
			return nil
		}
		if expectedNames[obj.GetName()] {
			return nil
		}

		log.Info("Deleting orphaned resource", "kind", resourceKind, "name", obj.GetName())
		if err := c.Delete(ctx, obj); err != nil && !apierr.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("fails to delete orphaned %s %s: %w", resourceKind, obj.GetName(), err))
		}
		return nil
	}); err != nil {
		errs = append(errs, fmt.Errorf("fails to iterate %s resources for cleanup: %w", resourceKind, err))
	}

	return utilerrors.NewAggregate(errs)
}
