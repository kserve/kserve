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

package kernelcachecommon

const (
	// Finalizer names
	KernelCacheFinalizerName = "kernelcache.kserve.io/finalizer"

	// Container and volume names
	ExtractContainerName = "kserve-kernelcache-extract"
	CachePVCMountName    = "cache-pvc"

	// Mount paths
	MountPath = "/mnt/kernel-cache"

	// Default values
	DefaultJobNamespace                     = "kserve"
	DefaultExtractImage                     = "quay.io/gkm/gkm-extract:latest"
	DefaultJobTTLSecondsAfterFinished int32 = 3600
	DefaultReconcileIntervalSeconds   int64 = 60

	// Minimum TTL to allow state propagation from Job completion to KernelCacheNode agents
	// Must be > 2x reconcile interval (60s) to ensure at least 2 reconcile cycles
	MinJobTTLSecondsAfterFinished int32 = 120
)
