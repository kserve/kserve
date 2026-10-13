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
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/utils"
)

// New P/D presets opt into multi-port routing with this empty env slot and a
// named modelserver port. Older pinned presets retain their headless topology.
// Like the KV transfer slot, this is a versioned preset/controller contract:
// changing the generated arguments or topology requires a new slot name.
const dpRoutingArgsEnvVar = "KSERVE_DP_ROUTING_ARGS"

const (
	dpHealthPortName              = "modelserver"
	dpActivePortsAnnotation       = "llm-d.ai/active-ports"
	dpMultiPortFlag               = "--data-parallel-multi-port-external-lb"
	dpMaxLocalRanks         int32 = 8 // InferencePool v1 permits at most eight targetPorts.
)

type dataParallelRoutingError struct{ error }

func applyDataParallelRouting(cfg *v1alpha2.LLMInferenceServiceConfig, userSpec v1alpha2.LLMInferenceServiceSpec) error {
	spec := &cfg.Spec
	maxRanks := int32(1)
	for _, workload := range []*v1alpha2.WorkloadSpec{&spec.WorkloadSpec, spec.Prefill} {
		if workload == nil {
			continue
		}
		localRanks := int32(1)
		if workload.Parallelism != nil {
			localRanks = max(ptr.Deref(workload.Parallelism.DataLocal, 1), 1)
		}
		for _, pod := range []*corev1.PodSpec{workload.Template, workload.Worker} {
			if err := configureDataParallelPod(pod, localRanks); err != nil {
				return err
			}
			if hasMultiPortDataParallel(pod) {
				maxRanks = max(maxRanks, localRanks)
			}
		}
		if hasMultiPortDataParallel(workload.Worker) && workload.Template != nil {
			leader := utils.GetContainerWithName(workload.Template, mainContainerName)
			worker := utils.GetContainerWithName(workload.Worker, mainContainerName)
			// Headless workers had no HTTP probes. Every serving worker now needs
			// the same supervisor checks, including the configured TLS scheme.
			if leader != nil {
				if worker.StartupProbe == nil {
					worker.StartupProbe = leader.StartupProbe.DeepCopy()
				}
				if worker.ReadinessProbe == nil {
					worker.ReadinessProbe = leader.ReadinessProbe.DeepCopy()
				}
				if worker.LivenessProbe == nil {
					worker.LivenessProbe = leader.LivenessProbe.DeepCopy()
				}
			}
			// Decode workers need their own proxy for their local ranks. Keep a
			// worker-specific sidecar if the user supplied one.
			if sidecar := routingSidecar(workload.Template); sidecar != nil && routingSidecar(workload.Worker) == nil {
				workload.Worker.InitContainers = append(workload.Worker.InitContainers, *sidecar.DeepCopy())
			}
		}
	}
	if maxRanks == 1 || spec.Router == nil || spec.Router.Scheduler == nil {
		return nil
	}
	pool := spec.Router.Scheduler.Pool
	if pool == nil || pool.HasRef() || pool.Spec == nil {
		return nil
	}
	// User/baseRef ports and customized scheduler presets take precedence.
	if userSpec.Router != nil && userSpec.Router.Scheduler != nil {
		userPool := userSpec.Router.Scheduler.Pool
		if userPool != nil && userPool.Spec != nil && len(userPool.Spec.TargetPorts) > 0 {
			return nil
		}
	}
	if !slices.Equal(pool.Spec.TargetPorts, []igwapi.Port{{Number: 8000}}) {
		return nil
	}
	pool.Spec.TargetPorts = make([]igwapi.Port, maxRanks)
	for rank := range maxRanks {
		pool.Spec.TargetPorts[rank] = igwapi.Port{Number: igwapi.PortNumber(8000 + rank)}
	}
	return nil
}

func configureDataParallelPod(pod *corev1.PodSpec, localRanks int32) error {
	if pod == nil {
		return nil
	}
	main := utils.GetContainerWithName(pod, mainContainerName)
	if main == nil {
		return nil
	}
	slot := slices.IndexFunc(main.Env, func(env corev1.EnvVar) bool { return env.Name == dpRoutingArgsEnvVar })
	if slot < 0 {
		return nil
	}
	if main.Env[slot].ValueFrom != nil || main.Env[slot].Value != "" {
		return fmt.Errorf("%s must be empty; the controller derives it from parallelism.dataLocal", dpRoutingArgsEnvVar)
	}
	if localRanks <= 1 {
		return nil // vLLM's multi-port supervisor requires at least two local ranks.
	}
	if localRanks > dpMaxLocalRanks {
		return fmt.Errorf("multi-port routing supports at most 8 local data-parallel ranks, got %d", localRanks)
	}
	health := slices.IndexFunc(main.Ports, func(port corev1.ContainerPort) bool { return port.Name == dpHealthPortName })
	if health < 0 {
		return fmt.Errorf("%s requires a named %s container port", dpRoutingArgsEnvVar, dpHealthPortName)
	}
	basePort := main.Ports[health].ContainerPort
	main.Ports[health].ContainerPort = basePort + dpMaxLocalRanks
	main.Env[slot].Value = fmt.Sprintf("%s --data-parallel-supervisor-port=%d", dpMultiPortFlag, basePort+dpMaxLocalRanks)
	appendDataParallelPorts(main, basePort, localRanks, "dp-rank")
	if main.ReadinessProbe != nil && main.ReadinessProbe.HTTPGet != nil && main.ReadinessProbe.HTTPGet.Port.StrVal == dpHealthPortName {
		main.ReadinessProbe.HTTPGet.Path = "/readyz"
	}
	if sidecar := routingSidecar(pod); sidecar != nil {
		appendDataParallelPorts(sidecar, 8000, localRanks, "dp-proxy")
	}
	return nil
}

func appendDataParallelPorts(container *corev1.Container, base, count int32, prefix string) {
	for rank := range count {
		port := base + rank
		if !slices.ContainsFunc(container.Ports, func(p corev1.ContainerPort) bool { return p.ContainerPort == port }) {
			container.Ports = append(container.Ports, corev1.ContainerPort{
				Name: fmt.Sprintf("%s-%d", prefix, rank), ContainerPort: port, Protocol: corev1.ProtocolTCP,
			})
		}
	}
}

func hasMultiPortDataParallel(pod *corev1.PodSpec) bool {
	if pod == nil {
		return false
	}
	main := utils.GetContainerWithName(pod, mainContainerName)
	if main == nil {
		return false
	}
	return slices.ContainsFunc(main.Env, func(env corev1.EnvVar) bool {
		return env.Name == dpRoutingArgsEnvVar && slices.Contains(strings.Fields(env.Value), dpMultiPortFlag)
	})
}

// The pool contains the union of prefill/decode rank ports. Tell EPP which ports
// each pod actually serves, including the one-port half of an asymmetric P/D
// deployment. An explicit per-workload annotation remains authoritative.
func applyDataParallelActivePorts(svc *v1alpha2.LLMInferenceService, workload *v1alpha2.WorkloadSpec, template *corev1.PodTemplateSpec) {
	if template == nil {
		return
	}
	enabled := hasMultiPortDataParallel(svc.Spec.Template) || hasMultiPortDataParallel(svc.Spec.Worker)
	if prefill := svc.Spec.Prefill; prefill != nil {
		enabled = enabled || hasMultiPortDataParallel(prefill.Template) || hasMultiPortDataParallel(prefill.Worker)
	}
	if !enabled {
		return
	}
	if _, explicit := template.Annotations[dpActivePortsAnnotation]; explicit {
		return
	}
	ports := []string{"8000"}
	if hasMultiPortDataParallel(&template.Spec) && workload != nil && workload.Parallelism != nil {
		ports = nil
		for rank := range ptr.Deref(workload.Parallelism.DataLocal, 1) {
			ports = append(ports, strconv.Itoa(8000+int(rank)))
		}
	}
	if template.Annotations == nil {
		template.Annotations = make(map[string]string)
	}
	template.Annotations[dpActivePortsAnnotation] = strings.Join(ports, ",")
}
