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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

func TestDataParallelRouting(t *testing.T) {
	for _, tls := range []bool{false, true} {
		for _, sizes := range [][2]int32{{8, 8}, {2, 8}, {8, 2}, {1, 8}, {8, 1}, {1, 1}} {
			t.Run(fmt.Sprintf("prefill%d-decode%d-tls%t", sizes[0], sizes[1], tls), func(t *testing.T) {
				svc, r := dataParallelRoutingFixture(t, sizes[0], sizes[1])
				combined, err := r.combineBaseRefsConfig(t.Context(), svc, &Config{EnableTLS: tls})
				require.NoError(t, err)
				spec := &combined.Config.Spec
				ports := spec.Router.Scheduler.Pool.Spec.TargetPorts
				require.Len(t, ports, int(max(sizes[0], sizes[1])))
				for i, port := range ports {
					assert.EqualValues(t, 8000+i, port.Number)
				}
				for i, workload := range []*v1alpha2.WorkloadSpec{spec.Prefill, &spec.WorkloadSpec} {
					basePort := int32(8000)
					if i == 1 {
						basePort = 8200
					}
					for _, pod := range []*corev1.PodSpec{workload.Template, workload.Worker} {
						main := utils.GetContainerWithName(pod, "main")
						require.NotNil(t, main)
						if sidecar := routingSidecar(pod); sidecar != nil {
							group := constants.InferencePoolV1APIGroupName
							assert.Contains(t, sidecar.Command, "--pool-group="+group)
						}
						args := strings.Join(main.Command, " ")
						assert.NotContains(t, args, "KSERVE_DP_ROUTING_ARGS")
						assert.NotContains(t, args, "--headless")
						if sizes[i] > 1 {
							assert.Contains(t, args, "--data-parallel-multi-port-external-lb")
							assert.Contains(t, args, fmt.Sprintf("--data-parallel-supervisor-port=%d", basePort+8))
						} else {
							assert.Contains(t, args, "--data-parallel-rank")
						}
						require.NotNil(t, main.ReadinessProbe)
						if sizes[i] > 1 {
							assert.Equal(t, "dp-supervisor", main.ReadinessProbe.HTTPGet.Port.StrVal)
							assert.Equal(t, "/readyz", main.ReadinessProbe.HTTPGet.Path)
						} else {
							assert.Equal(t, "dp-rank-0", main.ReadinessProbe.HTTPGet.Port.StrVal)
							assert.Equal(t, "/health", main.ReadinessProbe.HTTPGet.Path)
						}
						for rank := range sizes[i] {
							name := fmt.Sprintf("dp-rank-%d", rank)
							assert.Contains(t, main.Ports, corev1.ContainerPort{Name: name, ContainerPort: basePort + rank, Protocol: corev1.ProtocolTCP})
						}
						assert.Contains(t, main.Ports, corev1.ContainerPort{Name: "dp-supervisor", ContainerPort: basePort + 8, Protocol: corev1.ProtocolTCP})
						for _, probe := range []*corev1.Probe{main.StartupProbe, main.LivenessProbe} {
							require.NotNil(t, probe)
							require.NotNil(t, probe.HTTPGet)
							assert.Equal(t, main.ReadinessProbe.HTTPGet.Port, probe.HTTPGet.Port)
							assert.Equal(t, "/health", probe.HTTPGet.Path)
						}
						if i == 1 {
							sidecar := routingSidecar(pod)
							require.NotNil(t, sidecar, "decode workers must also proxy their local ranks")
							assert.Contains(t, sidecar.Command, fmt.Sprintf("--data-parallel-size=%d", sizes[i]))
							assert.Contains(t, sidecar.Command, "--model-server-port=8200")
							assert.Contains(t, sidecar.Command, "--pool-group=inference.networking.k8s.io")
							assert.Len(t, sidecar.Ports, 8)
							assert.Contains(t, sidecar.Ports, corev1.ContainerPort{Name: "routing-sidecar", ContainerPort: 8000, Protocol: corev1.ProtocolTCP})
							assert.Equal(t, "routing-sidecar", sidecar.LivenessProbe.HTTPGet.Port.StrVal)
							assert.Equal(t, "routing-sidecar", sidecar.ReadinessProbe.HTTPGet.Port.StrVal)
						} else {
							assert.Nil(t, routingSidecar(pod), "prefill serves rank ports directly")
						}
						wantScheme := corev1.URISchemeHTTP
						if tls {
							wantScheme = corev1.URISchemeHTTPS
						}
						assert.Equal(t, wantScheme, main.ReadinessProbe.HTTPGet.Scheme)
					}
				}

				// These templates are shared by LeaderWorkerSet and DisaggregatedSet.
				svc.Spec = *spec
				renderer := disaggTestReconciler(t)
				cfg := disaggTestConfig()
				decode, err := renderer.expectedMultiNodeMainLeaderWorkerTemplate(t.Context(), svc, cfg, nil, corev1.PodSpec{})
				require.NoError(t, err)
				prefill, err := renderer.expectedMultiNodePrefillLeaderWorkerTemplate(t.Context(), svc, cfg, nil, corev1.PodSpec{})
				require.NoError(t, err)
				for i, group := range []*multiNodeTemplate{prefill, decode} {
					assert.Equal(t, constants.KServeComponentWorkload, group.Template.WorkerTemplate.Labels[constants.KServeComponentLabelKey])
					var active []string
					for rank := range sizes[i] {
						active = append(active, strconv.Itoa(8000+int(rank)))
					}
					assert.Equal(t, strings.Join(active, ","), group.Template.LeaderTemplate.Annotations["llm-d.ai/active-ports"])
					assert.Equal(t, strings.Join(active, ","), group.Template.WorkerTemplate.Annotations["llm-d.ai/active-ports"])
				}
			})
		}
	}
}

func TestSingleNodeSidecarUsesV1PoolAPI(t *testing.T) {
	svc, r := dataParallelRoutingFixture(t, 1, 1)
	for _, name := range []string{"config-llm-decode-template.yaml", "config-llm-prefill-template.yaml"} {
		preset := loadPresetConfig(t, name)
		preset.ObjectMeta = metav1.ObjectMeta{Name: preset.Name, Namespace: svc.Namespace}
		require.NoError(t, r.Create(t.Context(), preset))
	}
	svc.Spec.BaseRefs = nil
	svc.Spec.Worker = nil
	svc.Spec.Parallelism = nil
	svc.Spec.Prefill.Worker = nil
	svc.Spec.Prefill.Parallelism = nil

	combined, err := r.combineBaseRefsConfig(t.Context(), svc, &Config{})
	require.NoError(t, err)
	require.Equal(t, []igwapi.Port{{Number: 8000}}, combined.Config.Spec.Router.Scheduler.Pool.Spec.TargetPorts)
	sidecar := routingSidecar(combined.Config.Spec.Template)
	require.NotNil(t, sidecar)
	assert.Contains(t, sidecar.Command, "--pool-group="+constants.InferencePoolV1APIGroupName)
}

func TestDataParallelRoutingPreservesExplicitPoolPorts(t *testing.T) {
	svc, r := dataParallelRoutingFixture(t, 2, 8)
	want := []igwapi.Port{{Number: 8000}, {Number: 8001}}
	svc.Spec.Router.Scheduler.Pool = &v1alpha2.InferencePoolSpec{Spec: &igwapi.InferencePoolSpec{TargetPorts: want}}
	combined, err := r.combineBaseRefsConfig(t.Context(), svc, &Config{})
	require.NoError(t, err)
	assert.Equal(t, want, combined.Config.Spec.Router.Scheduler.Pool.Spec.TargetPorts)
}

func TestDataParallelRoutingFromBaseRef(t *testing.T) {
	svc, r := dataParallelRoutingFixture(t, 2, 8)
	base := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "wide-ep", Namespace: svc.Namespace},
		Spec:       *svc.Spec.DeepCopy(),
	}
	// Keep all parallelism and workload settings in a baseRef, then override
	// decode's local/global count in the service. Routing must follow the final
	// merged topology, including the sidecar's size and the pool's port union.
	require.NoError(t, r.Create(t.Context(), base))
	svc.Spec = v1alpha2.LLMInferenceServiceSpec{
		BaseRefs: append(svc.Spec.BaseRefs, corev1.LocalObjectReference{Name: base.Name}),
		WorkloadSpec: v1alpha2.WorkloadSpec{
			Parallelism: &v1alpha2.ParallelismSpec{Data: ptr.To[int32](8), DataLocal: ptr.To[int32](4)},
		},
	}
	combined, err := r.combineBaseRefsConfig(t.Context(), svc, &Config{})
	require.NoError(t, err)
	assert.Len(t, combined.Config.Spec.Router.Scheduler.Pool.Spec.TargetPorts, 4)
	assert.Contains(t, routingSidecar(combined.Config.Spec.Worker).Command, "--data-parallel-size=4")
}

func TestDataParallelRoutingReferencedPool(t *testing.T) {
	svc, r := dataParallelRoutingFixture(t, 8, 8)
	svc.Spec.Router.Scheduler.Pool = &v1alpha2.InferencePoolSpec{Ref: &corev1.LocalObjectReference{Name: "external-pool"}}
	combined, err := r.combineBaseRefsConfig(t.Context(), svc, &Config{})
	require.NoError(t, err)
	assert.Nil(t, combined.Config.Spec.Router.Scheduler.Pool.Spec)
	assert.Equal(t, "external-pool", combined.Config.Spec.Router.Scheduler.Pool.Ref.Name)
	assert.Contains(t, routingSidecar(combined.Config.Spec.Worker).Command, "--inference-pool="+svc.Namespace+"/external-pool")
	assert.Contains(t, routingSidecar(combined.Config.Spec.Worker).Command, "--pool-group="+constants.InferencePoolV1APIGroupName)
}

func TestDataParallelRoutingRequiresBaseRefs(t *testing.T) {
	svc, r := dataParallelRoutingFixture(t, 8, 8)
	svc.Spec.BaseRefs = nil
	combined, err := r.combineBaseRefsConfig(t.Context(), svc, &Config{})
	require.NoError(t, err)
	require.Equal(t, []igwapi.Port{{Number: 8000}}, combined.Config.Spec.Router.Scheduler.Pool.Spec.TargetPorts)
	require.Nil(t, routingSidecar(combined.Config.Spec.Worker))
	main := utils.GetContainerWithName(combined.Config.Spec.Worker, mainContainerName)
	require.NotNil(t, main)
	require.Nil(t, main.ReadinessProbe)
	assert.Contains(t, strings.Join(main.Command, " "), "--headless")
}

func TestDataParallelBaseRefsReplaceDefaults(t *testing.T) {
	svc, r := dataParallelRoutingFixture(t, 2, 4)
	// Opted-in services must not even load the old topology. This protects against
	// old preset removal and against accidentally merging arbitrary old fields.
	for _, name := range []string{"kserve-config-llm-decode-worker-data-parallel", "kserve-config-llm-prefill-worker-data-parallel"} {
		old := &v1alpha2.LLMInferenceServiceConfig{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: svc.Namespace}}
		require.NoError(t, r.Delete(t.Context(), old))
	}
	combined, err := r.combineBaseRefsConfig(t.Context(), svc, &Config{})
	require.NoError(t, err)
	for _, ref := range combined.AppliedConfigRefs {
		assert.NotEqual(t, "kserve-config-llm-decode-worker-data-parallel", string(ref.Name))
		assert.NotEqual(t, "kserve-config-llm-prefill-worker-data-parallel", string(ref.Name))
	}
}

func TestDataParallelRoutingPreservesActivePorts(t *testing.T) {
	for _, selected := range []string{"both", "prefill", "decode"} {
		t.Run(selected, func(t *testing.T) {
			svc, r := dataParallelRoutingFixture(t, 1, 4)
			if selected != "both" {
				svc.Spec.BaseRefs = []corev1.LocalObjectReference{{Name: "kserve-config-llm-" + selected + "-worker-data-parallel-multi-port"}}
			}
			svc.Spec.Prefill.Annotations = map[string]string{dpActivePortsAnnotation: "9000"}
			svc.Spec.Annotations[dpActivePortsAnnotation] = "9001,9003"
			combined, err := r.combineBaseRefsConfig(t.Context(), svc, &Config{})
			require.NoError(t, err)
			assert.Equal(t, "9000", combined.Config.Spec.Prefill.Annotations[dpActivePortsAnnotation])
			assert.Equal(t, "9001,9003", combined.Config.Spec.Annotations[dpActivePortsAnnotation])
		})
	}
}

func TestCustomMultiPortPoolDoesNotDefaultActivePorts(t *testing.T) {
	for _, tc := range []struct {
		name          string
		ports         []igwapi.Port
		decodeActive  string
		prefillActive string
	}{
		{name: "custom traffic ports", ports: []igwapi.Port{{Number: 9000}, {Number: 9001}}},
		{name: "standard traffic ports", ports: []igwapi.Port{{Number: 8000}, {Number: 8001}}},
		{name: "explicit decode ports", ports: []igwapi.Port{{Number: 9000}, {Number: 9001}}, decodeActive: "9001"},
		{name: "explicit prefill ports", ports: []igwapi.Port{{Number: 9000}, {Number: 9001}}, prefillActive: "9000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, r := dataParallelRoutingFixture(t, 2, 4)
			svc.Spec.BaseRefs = nil // A custom pool does not opt into the new topology.
			svc.Spec.Router.Scheduler.Pool = &v1alpha2.InferencePoolSpec{Spec: &igwapi.InferencePoolSpec{TargetPorts: tc.ports}}
			if tc.decodeActive != "" {
				svc.Spec.Annotations[dpActivePortsAnnotation] = tc.decodeActive
			}
			if tc.prefillActive != "" {
				svc.Spec.Prefill.Annotations[dpActivePortsAnnotation] = tc.prefillActive
			}
			combined, err := r.combineBaseRefsConfig(t.Context(), svc, &Config{})
			require.NoError(t, err)
			require.Equal(t, tc.ports, combined.Config.Spec.Router.Scheduler.Pool.Spec.TargetPorts)
			svc.Spec = combined.Config.Spec
			ds, err := disaggTestReconciler(t).expectedDisaggregatedSet(t.Context(), svc, disaggTestConfig(), nil)
			require.NoError(t, err)
			for _, role := range ds.Spec.Roles {
				want := tc.decodeActive
				if role.Name == constants.LLMDRolePrefill {
					want = tc.prefillActive
				}
				group := role.Spec.LeaderWorkerTemplate
				for _, pod := range []*corev1.PodTemplateSpec{group.LeaderTemplate, &group.WorkerTemplate} {
					if want == "" {
						assert.NotContains(t, pod.Annotations, dpActivePortsAnnotation)
					} else {
						assert.Equal(t, want, pod.Annotations[dpActivePortsAnnotation])
					}
				}
			}
		})
	}
}

func TestDataParallelBaseRefsSelectRolesIndependently(t *testing.T) {
	for _, role := range []string{"decode", "prefill"} {
		t.Run(role, func(t *testing.T) {
			svc, r := dataParallelRoutingFixture(t, 2, 4)
			svc.Spec.BaseRefs = []corev1.LocalObjectReference{{Name: "kserve-config-llm-" + role + "-worker-data-parallel-multi-port"}}
			combined, err := r.combineBaseRefsConfig(t.Context(), svc, &Config{})
			require.NoError(t, err)
			wantPorts := 2
			if role == "decode" {
				wantPorts = 4
			}
			require.Len(t, combined.Config.Spec.Router.Scheduler.Pool.Spec.TargetPorts, wantPorts)
			svc.Spec = combined.Config.Spec
			renderer := disaggTestReconciler(t)
			cfg := disaggTestConfig()
			ds, err := renderer.expectedDisaggregatedSet(t.Context(), svc, cfg, nil)
			require.NoError(t, err)
			for _, gotRole := range ds.Spec.Roles {
				worker := gotRole.Spec.LeaderWorkerTemplate.WorkerTemplate
				if gotRole.Name == role {
					require.Equal(t, constants.KServeComponentWorkload, worker.Labels[constants.KServeComponentLabelKey])
				} else {
					require.Empty(t, worker.Labels[constants.KServeComponentLabelKey])
					require.Equal(t, "8000", gotRole.Spec.LeaderWorkerTemplate.LeaderTemplate.Annotations[dpActivePortsAnnotation])
				}
			}
		})
	}
}

func TestDataParallelBaseRefAllowsRenamedCopy(t *testing.T) {
	svc, r := dataParallelRoutingFixture(t, 2, 4)
	for i, ref := range svc.Spec.BaseRefs {
		cfg := &v1alpha2.LLMInferenceServiceConfig{}
		require.NoError(t, r.Get(t.Context(), client.ObjectKey{Namespace: svc.Namespace, Name: ref.Name}, cfg))
		cfg.Name = "custom-" + ref.Name
		cfg.ResourceVersion = ""
		require.NoError(t, r.Create(t.Context(), cfg))
		svc.Spec.BaseRefs[i].Name = cfg.Name
	}
	combined, err := r.combineBaseRefsConfig(t.Context(), svc, &Config{})
	require.NoError(t, err)
	require.Len(t, combined.Config.Spec.Router.Scheduler.Pool.Spec.TargetPorts, 4)
	for _, ref := range combined.AppliedConfigRefs {
		assert.NotEqual(t, "kserve-config-llm-decode-worker-data-parallel", string(ref.Name))
		assert.NotEqual(t, "kserve-config-llm-prefill-worker-data-parallel", string(ref.Name))
	}
}

func TestDataParallelBaseRefWithTensorParallelPrefill(t *testing.T) {
	svc, r := dataParallelRoutingFixture(t, 1, 4)
	svc.Spec.BaseRefs = svc.Spec.BaseRefs[1:]
	svc.Spec.Prefill.Worker = nil
	svc.Spec.Prefill.Parallelism = &v1alpha2.ParallelismSpec{Tensor: ptr.To[int32](8), Expert: true}
	preset := loadPresetConfig(t, "config-llm-prefill-template.yaml")
	preset.Namespace = svc.Namespace
	require.NoError(t, r.Create(t.Context(), preset))
	combined, err := r.combineBaseRefsConfig(t.Context(), svc, &Config{})
	require.NoError(t, err)
	require.Len(t, combined.Config.Spec.Router.Scheduler.Pool.Spec.TargetPorts, 4)
	require.Nil(t, combined.Config.Spec.Prefill.Worker)
	main := utils.GetContainerWithName(combined.Config.Spec.Prefill.Template, mainContainerName)
	require.NotNil(t, main)
	assert.Contains(t, strings.Join(main.Command, " "), "--tensor-parallel-size 8")
	svc.Spec = combined.Config.Spec
	pod, err := disaggTestReconciler(t).expectedSingleNodePrefillPodTemplate(t.Context(), svc, disaggTestConfig(), corev1.PodSpec{})
	require.NoError(t, err)
	assert.Equal(t, "8000", pod.Template.Annotations[dpActivePortsAnnotation])
}

func TestDataParallelBaseRefRejectsInvalidReplacement(t *testing.T) {
	svc, r := dataParallelRoutingFixture(t, 2, 4)
	cfg := &v1alpha2.LLMInferenceServiceConfig{}
	require.NoError(t, r.Get(t.Context(), client.ObjectKey{Namespace: svc.Namespace, Name: svc.Spec.BaseRefs[0].Name}, cfg))
	cfg.Annotations[configReplacesPresetAnnotation] = "config-llm-scheduler"
	require.NoError(t, r.Update(t.Context(), cfg))
	_, err := r.reconcileBaseRefs(t.Context(), svc, &Config{})
	require.ErrorContains(t, err, "unsupported serving.kserve.io/replaces-preset")
	assert.ErrorIs(t, err, reconcile.TerminalError(nil))
}

func TestDataParallelPoolPortsFromBaseRefTakePrecedence(t *testing.T) {
	svc, r := dataParallelRoutingFixture(t, 2, 4)
	want := []igwapi.Port{{Number: 8000}, {Number: 8002}}
	custom := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "custom-pool", Namespace: svc.Namespace},
		Spec: v1alpha2.LLMInferenceServiceSpec{Router: &v1alpha2.RouterSpec{Scheduler: &v1alpha2.SchedulerSpec{
			Pool: &v1alpha2.InferencePoolSpec{Spec: &igwapi.InferencePoolSpec{TargetPorts: want}},
		}}},
	}
	require.NoError(t, r.Create(t.Context(), custom))
	svc.Spec.BaseRefs = append(svc.Spec.BaseRefs, corev1.LocalObjectReference{Name: custom.Name})
	combined, err := r.combineBaseRefsConfig(t.Context(), svc, &Config{})
	require.NoError(t, err)
	assert.Equal(t, want, combined.Config.Spec.Router.Scheduler.Pool.Spec.TargetPorts)
}

func TestDataParallelBaseRefsRetainPresetKVCacheSizing(t *testing.T) {
	svc, r := dataParallelRoutingFixture(t, 2, 4)
	for i, ref := range svc.Spec.BaseRefs {
		preset := &v1alpha2.LLMInferenceServiceConfig{}
		require.NoError(t, r.Get(t.Context(), client.ObjectKey{Namespace: svc.Namespace, Name: ref.Name}, preset))
		require.NoError(t, r.Delete(t.Context(), preset))
		preset.Namespace = constants.KServeNamespace
		// A subsequent controller release must still recognize a config name
		// explicitly pinned to an earlier release through baseRefs.
		preset.Name = "release-v1-" + preset.Name
		preset.ResourceVersion = ""
		require.NoError(t, r.Create(t.Context(), preset))
		svc.Spec.BaseRefs[i].Name = preset.Name
	}
	svc.Spec.KVCacheOffloading = &v1alpha2.KVCacheOffloadingSpec{CPU: resource.MustParse("10Gi")}
	svc.Spec.Prefill.KVCacheOffloading = &v1alpha2.KVCacheOffloadingSpec{CPU: resource.MustParse("10Gi")}
	combined, err := r.combineBaseRefsConfig(t.Context(), svc, &Config{})
	require.NoError(t, err)
	for _, workload := range []*v1alpha2.WorkloadSpec{&combined.Config.Spec.WorkloadSpec, combined.Config.Spec.Prefill} {
		for _, pod := range []*corev1.PodSpec{workload.Template, workload.Worker} {
			for _, volume := range pod.Volumes {
				if volume.Name == "dshm" {
					require.NotNil(t, volume.EmptyDir)
					require.NotNil(t, volume.EmptyDir.SizeLimit)
					assert.Zero(t, volume.EmptyDir.SizeLimit.Cmp(resource.MustParse("20Gi")))
				}
			}
		}
	}
}

// Execute the rendered launch command, substituting only the vllm executable.
// Both single-rank and multi-port launch modes must serve on every worker,
// without --headless or --api-server-count inherited from the old topology.
func TestDataParallelRoutingLaunchArguments(t *testing.T) {
	for _, local := range []int32{1, 8} {
		t.Run(strconv.Itoa(int(local)), func(t *testing.T) {
			svc, r := dataParallelRoutingFixture(t, local, local)
			combined, err := r.combineBaseRefsConfig(t.Context(), svc, &Config{})
			require.NoError(t, err)
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "vllm"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0o755)) //nolint:gosec // executable test stub
			for _, workload := range []*v1alpha2.WorkloadSpec{&combined.Config.Spec.WorkloadSpec, combined.Config.Spec.Prefill} {
				for _, pod := range []*corev1.PodSpec{workload.Template, workload.Worker} {
					main := utils.GetContainerWithName(pod, "main")
					script := main.Command[2]
					start := strings.Index(script, "eval \"exec vllm serve")
					require.GreaterOrEqual(t, start, 0)
					cmd := exec.CommandContext(t.Context(), "/bin/bash", "-c", script[start:]) // #nosec G204 -- rendered test fixture
					cmd.Env = []string{"PATH=" + dir + ":/usr/bin:/bin", "VLLM_API_SERVER_COUNT=8", "DP_ADDRESS=127.0.0.1", "START_RANK=0"}

					out, err := cmd.CombinedOutput()
					require.NoError(t, err, "%s", out)
					args := strings.Fields(string(out))
					switch {
					case local > 1:
						assert.Contains(t, args, "--data-parallel-multi-port-external-lb")
						assert.NotContains(t, args, "--headless")
						assert.NotContains(t, args, "--api-server-count")
					default:
						assert.Contains(t, args, "--data-parallel-rank")
					}
					assert.NotContains(t, args, "--headless")
					assert.NotContains(t, args, "--api-server-count")
				}
			}
		})
	}
}

func TestDataParallelRoutingRejectsTooManyLocalRanks(t *testing.T) {
	svc, r := dataParallelRoutingFixture(t, 8, 16)
	_, err := r.reconcileBaseRefs(t.Context(), svc, &Config{})
	require.ErrorContains(t, err, "at most 8 local data-parallel ranks")
	assert.ErrorIs(t, err, reconcile.TerminalError(nil))
}

func TestDataParallelRoutingLegacySidecarFlagCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name            string
		schedulerVer    string
		command         []string
		wantTerminalErr bool
	}{
		{
			name:         "v1 annotation rejects removed vllm-port",
			schedulerVer: "1.0.0",
			command: []string{
				"/app/pd-sidecar",
				"--vllm-port=8200",
				"--pool-group=inference.networking.x-k8s.io",
			},
			wantTerminalErr: true,
		},
		{
			name:         "v1 annotation lets legacy pool-group reach migration",
			schedulerVer: "1.0.0",
			command: []string{
				"/app/pd-sidecar",
				"--pool-group=inference.networking.x-k8s.io",
			},
		},
		{
			name:         "pre-v1 annotation keeps legacy flags",
			schedulerVer: "0.11.0",
			command: []string{
				"/app/pd-sidecar",
				"--vllm-port=8200",
				"--pool-group=inference.networking.x-k8s.io",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, r := dataParallelRoutingFixture(t, 1, 1)
			r.EventRecorder = record.NewFakeRecorder(10)
			svc.Spec.Router.Scheduler.Annotations = map[string]string{"app.kubernetes.io/version": tc.schedulerVer}
			svc.Spec.Worker.InitContainers = []corev1.Container{{
				Name:    constants.LLMISVCRoutingSidecarContainerName,
				Image:   "sidecar:latest",
				Command: tc.command,
			}}

			_, err := r.reconcileBaseRefs(t.Context(), svc, &Config{})

			if !tc.wantTerminalErr {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, reconcile.TerminalError(nil))
			require.ErrorContains(t, err, "replace --vllm-port with --model-server-port")
			require.ErrorContains(t, err, "use llm-d-router-disagg-sidecar v1.0.0 or newer")
			require.NotContains(t, err.Error(), "set --pool-group=inference.networking.k8s.io")
			condition := svc.Status.GetCondition(v1alpha2.PresetsCombined)
			require.NotNil(t, condition)
			assert.Equal(t, "RoutingSidecarArgsInvalid", condition.Reason)
			select {
			case event := <-r.EventRecorder.(*record.FakeRecorder).Events:
				assert.Contains(t, event, "RoutingSidecarArgsInvalid")
			default:
				t.Fatal("expected a routing-sidecar compatibility event")
			}
		})
	}
}

func dataParallelRoutingFixture(t *testing.T, prefill, decode int32) (*v1alpha2.LLMInferenceService, *LLMISVCReconciler) {
	t.Helper()
	svc := disaggTestService(t)
	svc.Spec.Model.Name = ptr.To("org/model")
	svc.Spec.Worker = &corev1.PodSpec{}
	svc.Spec.Template = nil
	svc.Spec.Parallelism = &v1alpha2.ParallelismSpec{Data: ptr.To(2 * decode), DataLocal: ptr.To(decode), Expert: true}
	svc.Spec.Prefill.Worker = &corev1.PodSpec{}
	svc.Spec.Prefill.Template = nil
	svc.Spec.Prefill.Parallelism = &v1alpha2.ParallelismSpec{Data: ptr.To(2 * prefill), DataLocal: ptr.To(prefill), Expert: true}
	svc.Spec.Router = &v1alpha2.RouterSpec{Scheduler: &v1alpha2.SchedulerSpec{}}
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))
	objects := make([]client.Object, 0, 4)
	for _, name := range []string{"config-llm-decode-worker-data-parallel.yaml", "config-llm-prefill-worker-data-parallel.yaml", "config-llm-decode-worker-data-parallel-multi-port.yaml", "config-llm-prefill-worker-data-parallel-multi-port.yaml", "config-llm-scheduler.yaml", "config-llm-scheduler-eppconfig-default-pd.yaml"} {
		preset := loadPresetConfig(t, name)
		preset.Namespace = svc.Namespace
		objects = append(objects, preset)
	}
	svc.Spec.BaseRefs = []corev1.LocalObjectReference{
		{Name: "kserve-config-llm-prefill-worker-data-parallel-multi-port"},
		{Name: "kserve-config-llm-decode-worker-data-parallel-multi-port"},
	}
	return svc, &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()}
}
