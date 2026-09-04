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

package main

import (
	"context"
	"crypto/tls"

	ctrl "sigs.k8s.io/controller-runtime"

	kservetls "github.com/kserve/kserve/pkg/tls"
)

func resolveTLS(_ context.Context, minVer, ciphers string) ([]func(*tls.Config), error) {
	return kservetls.Resolve(minVer, ciphers)
}

func setupDistroStartup(ctx context.Context, _ ctrl.Manager) (context.Context, error) {
	return ctx, nil
}
