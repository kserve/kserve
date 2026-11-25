/*
Copyright 2026 The KServe Authors.

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

package llmisvc_test

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

// reconcileUntilSettled drives Reconcile until it stops requesting a requeue or
// returns an error. The first reconciliation of a service dispatches its
// self-signed certificate generation to the background and requeues; child
// resources only appear once a later reconciliation finds the certificate
// Secret. Reconcilers passed here must inject fixture.CertificateKey so the
// generation settles in milliseconds rather than seconds.
func reconcileUntilSettled(ctx context.Context, r *llmisvc.LLMISVCReconciler, req reconcile.Request) (reconcile.Result, error) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		result, err := r.Reconcile(ctx, req)
		if err != nil || result.IsZero() {
			return result, err
		}
		if time.Now().After(deadline) {
			return result, err
		}
		time.Sleep(time.Millisecond)
	}
}
