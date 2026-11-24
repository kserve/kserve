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

package llmisvc

import (
	"fmt"
	"slices"

	"k8s.io/utils/ptr"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

const (
	// A baseRef config replaces one automatic DP preset. The value is its stable
	// suffix (without a release prefix), so renamed or versioned copies work too.
	// This belongs on the config's metadata, not on the service or its pods.
	configReplacesPresetAnnotation       = "serving.kserve.io/replaces-preset"
	dpActivePortsAnnotation              = "llm-d.ai/active-ports"
	dpMaxLocalRanks                int32 = 8 // InferencePool v1 permits at most eight targetPorts.
)

type dataParallelRoutingError struct{ error }

type dataParallelConfigSelection struct {
	decode  bool
	prefill bool
}

func (s *dataParallelConfigSelection) add(cfg *v1alpha2.LLMInferenceServiceConfig) error {
	var selected *bool
	switch replacement := cfg.Annotations[configReplacesPresetAnnotation]; replacement {
	case "":
		return nil
	case configDecodeWorkerDataParallelNameSuffix:
		selected = &s.decode
	case configPrefillWorkerDataParallelNameSuffix:
		selected = &s.prefill
	default:
		return fmt.Errorf("config %q has unsupported %s value %q", cfg.Name, configReplacesPresetAnnotation, replacement)
	}
	if *selected {
		return fmt.Errorf("multiple baseRefs replace the same data-parallel preset (config %q)", cfg.Name)
	}
	*selected = true
	return nil
}

// Commands, probes, sidecars and advertised rank ports live in the standalone
// configs. Only the pool-wide union needs computing from the final typed spec.
// Explicit pool ports (including those from other baseRefs) remain authoritative.
func (s *dataParallelConfigSelection) configurePool(cfg *v1alpha2.LLMInferenceServiceConfig, userSpec v1alpha2.LLMInferenceServiceSpec) error {
	maxRanks := int32(1)
	for _, role := range []struct {
		selected bool
		name     string
		workload *v1alpha2.WorkloadSpec
	}{
		{s.decode, "decode", &cfg.Spec.WorkloadSpec},
		{s.prefill, "prefill", cfg.Spec.Prefill},
	} {
		if !role.selected {
			continue
		}
		if role.workload == nil || role.workload.Worker == nil || !role.workload.Parallelism.IsDataParallel() {
			return fmt.Errorf("the %s per-rank config requires a data-parallel worker workload", role.name)
		}
		local := ptr.Deref(role.workload.Parallelism.DataLocal, 1)
		if local < 1 {
			return fmt.Errorf("per-rank routing requires at least one local data-parallel rank for %s", role.name)
		}
		if local > dpMaxLocalRanks {
			return fmt.Errorf("per-rank routing supports at most 8 local data-parallel ranks, got %d for %s", local, role.name)
		}
		maxRanks = max(maxRanks, local)
	}
	spec := &cfg.Spec
	if maxRanks == 1 || spec.Router == nil || spec.Router.Scheduler == nil {
		return nil
	}
	pool := spec.Router.Scheduler.Pool
	if pool == nil || pool.HasRef() || pool.Spec == nil {
		return nil
	}
	if userSpec.Router != nil && userSpec.Router.Scheduler != nil {
		declared := userSpec.Router.Scheduler.Pool
		if declared != nil && declared.Spec != nil && len(declared.Spec.TargetPorts) > 0 {
			return nil
		}
	}
	if !slices.Equal(pool.Spec.TargetPorts, []igwapi.Port{{Number: 8000}}) {
		return nil
	}
	// Prefill vLLM serves directly on 8000+rank. Decode's routing sidecar listens
	// on 8000+rank and forwards to vLLM on 8200+rank, so the pool targets
	// 8000+rank for both roles.
	pool.Spec.TargetPorts = make([]igwapi.Port, maxRanks)
	for rank := range maxRanks {
		pool.Spec.TargetPorts[rank] = igwapi.Port{Number: igwapi.PortNumber(8000 + rank)}
	}
	return nil
}

// The new configs explicitly label serving workers. This also allows an external
// pool's selector to be propagated without inferring topology from launch flags.
func workerServesDataParallelRanks(workload *v1alpha2.WorkloadSpec) bool {
	return workload != nil && workload.Labels[constants.KServeComponentLabelKey] == constants.KServeComponentWorkload
}

// Rank configs declare their own active-ports. Only when one is selected, give
// the other role's default single endpoint an explicit 8000 annotation so EPP
// does not register the pool's whole port union for it. Custom pool ports alone
// do not select this topology, and explicit workload annotations are preserved.
func (s *dataParallelConfigSelection) configureActivePorts(cfg *v1alpha2.LLMInferenceServiceConfig) {
	if !s.decode && !s.prefill {
		return
	}
	for _, role := range []struct {
		selected bool
		workload *v1alpha2.WorkloadSpec
	}{
		{s.decode, &cfg.Spec.WorkloadSpec},
		{s.prefill, cfg.Spec.Prefill},
	} {
		if role.selected || role.workload == nil {
			continue
		}
		if _, explicit := role.workload.Annotations[dpActivePortsAnnotation]; explicit {
			continue
		}
		if role.workload.Annotations == nil {
			role.workload.Annotations = make(map[string]string)
		}
		role.workload.Annotations[dpActivePortsAnnotation] = "8000"
	}
}
