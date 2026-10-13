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
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
					for j, pod := range []*corev1.PodSpec{workload.Template, workload.Worker} {
						main := utils.GetContainerWithName(pod, "main")
						require.NotNil(t, main)
						if sidecar := routingSidecar(pod); sidecar != nil {
							group := constants.InferencePoolV1Alpha2APIGroupName
							if len(ports) > 1 {
								group = constants.InferencePoolV1APIGroupName
							}
							assert.Contains(t, sidecar.Command, "--pool-group="+group)
						}
						var args string
						for _, env := range main.Env {
							if env.Name == "KSERVE_DP_ROUTING_ARGS" {
								args = env.Value
							}
						}
						if sizes[i] == 1 {
							assert.Empty(t, args, "one local rank retains its existing serving mode")
							if j == 1 {
								assert.Nil(t, main.ReadinessProbe, "headless worker has no HTTP endpoint")
								assert.Nil(t, routingSidecar(pod))
							}
							continue
						}
						assert.Contains(t, args, "--data-parallel-multi-port-external-lb")
						assert.Contains(t, args, fmt.Sprintf("--data-parallel-supervisor-port=%d", basePort+8))
						require.NotNil(t, main.ReadinessProbe)
						assert.Equal(t, "modelserver", main.ReadinessProbe.HTTPGet.Port.StrVal)
						assert.Equal(t, "/readyz", main.ReadinessProbe.HTTPGet.Path)
						for rank := range sizes[i] {
							assert.Contains(t, main.Ports, corev1.ContainerPort{Name: fmt.Sprintf("dp-rank-%d", rank), ContainerPort: basePort + rank, Protocol: corev1.ProtocolTCP})
						}
						assert.Contains(t, main.Ports, corev1.ContainerPort{Name: "modelserver", ContainerPort: basePort + 8, Protocol: corev1.ProtocolTCP})
						if i == 1 {
							sidecar := routingSidecar(pod)
							require.NotNil(t, sidecar, "decode workers must also proxy their local ranks")
							assert.Contains(t, sidecar.Command, fmt.Sprintf("--data-parallel-size=%d", sizes[i]))
							assert.Contains(t, sidecar.Command, "--model-server-port=8200")
							assert.Contains(t, sidecar.Command, "--pool-group=inference.networking.k8s.io")
							assert.Len(t, sidecar.Ports, int(sizes[i]))
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
					if sizes[i] > 1 {
						assert.Equal(t, constants.KServeComponentWorkload, group.Template.WorkerTemplate.Labels[constants.KServeComponentLabelKey])
					} else {
						assert.Empty(t, group.Template.WorkerTemplate.Labels[constants.KServeComponentLabelKey])
					}
					if max(sizes[0], sizes[1]) > 1 {
						var active []string
						for rank := range sizes[i] {
							active = append(active, strconv.Itoa(8000+int(rank)))
						}
						assert.Equal(t, strings.Join(active, ","), group.Template.LeaderTemplate.Annotations["llm-d.ai/active-ports"])
						if sizes[i] > 1 {
							assert.Equal(t, strings.Join(active, ","), group.Template.WorkerTemplate.Annotations["llm-d.ai/active-ports"])
						}
					}
				}
			})
		}
	}
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
		BaseRefs: []corev1.LocalObjectReference{{Name: base.Name}},
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

func TestDataParallelRoutingPinnedPreset(t *testing.T) {
	svc, r := dataParallelRoutingFixture(t, 8, 8)
	for _, name := range []string{"kserve-config-llm-decode-worker-data-parallel", "kserve-config-llm-prefill-worker-data-parallel"} {
		preset := &v1alpha2.LLMInferenceServiceConfig{}
		require.NoError(t, r.Get(t.Context(), client.ObjectKey{Namespace: svc.Namespace, Name: name}, preset))
		workload := &preset.Spec.WorkloadSpec
		if preset.Spec.Prefill != nil {
			workload = preset.Spec.Prefill
		}
		for _, pod := range []*corev1.PodSpec{workload.Template, workload.Worker} {
			main := utils.GetContainerWithName(pod, "main")
			main.Env = slices.DeleteFunc(main.Env, func(env corev1.EnvVar) bool { return env.Name == "KSERVE_DP_ROUTING_ARGS" })
		}
		require.NoError(t, r.Update(t.Context(), preset))
	}
	combined, err := r.combineBaseRefsConfig(t.Context(), svc, &Config{})
	require.NoError(t, err)
	assert.Equal(t, []igwapi.Port{{Number: 8000}}, combined.Config.Spec.Router.Scheduler.Pool.Spec.TargetPorts)
	assert.Nil(t, routingSidecar(combined.Config.Spec.Worker))
	assert.Nil(t, combined.Config.Spec.Worker.Containers[0].ReadinessProbe)
	assert.False(t, hasMultiPortDataParallel(combined.Config.Spec.Worker))
}

// Execute the rendered launch command, substituting only the vllm executable.
// In particular, exercise the nested shell fallback expansion: multi-port mode
// must never also launch with --headless or --api-server-count=8.
func TestDataParallelRoutingLaunchArguments(t *testing.T) {
	for _, local := range []int32{1, 8} {
		t.Run(strconv.Itoa(int(local)), func(t *testing.T) {
			svc, r := dataParallelRoutingFixture(t, local, local)
			combined, err := r.combineBaseRefsConfig(t.Context(), svc, &Config{})
			require.NoError(t, err)
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "vllm"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0o755)) //nolint:gosec // executable test stub
			for _, workload := range []*v1alpha2.WorkloadSpec{&combined.Config.Spec.WorkloadSpec, combined.Config.Spec.Prefill} {
				for worker, pod := range []*corev1.PodSpec{workload.Template, workload.Worker} {
					main := utils.GetContainerWithName(pod, "main")
					script := main.Command[2]
					start := strings.Index(script, "eval \"exec vllm serve")
					require.GreaterOrEqual(t, start, 0)
					cmd := exec.CommandContext(t.Context(), "/bin/bash", "-c", script[start:]) // #nosec G204 -- rendered test fixture
					cmd.Env = []string{"PATH=" + dir + ":/usr/bin:/bin", "VLLM_API_SERVER_COUNT=8", "DP_ADDRESS=127.0.0.1", "START_RANK=0"}
					for _, env := range main.Env {
						if env.Name == "KSERVE_DP_ROUTING_ARGS" {
							cmd.Env = append(cmd.Env, env.Name+"="+env.Value)
						}
					}
					out, err := cmd.CombinedOutput()
					require.NoError(t, err, "%s", out)
					args := strings.Fields(string(out))
					switch {
					case local > 1:
						assert.Contains(t, args, "--data-parallel-multi-port-external-lb")
						assert.NotContains(t, args, "--headless")
						assert.NotContains(t, args, "--api-server-count")
					case worker == 1:
						assert.Contains(t, args, "--headless")
					default:
						assert.Contains(t, args, "--api-server-count")
					}
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
	for _, name := range []string{"config-llm-decode-worker-data-parallel.yaml", "config-llm-prefill-worker-data-parallel.yaml", "config-llm-scheduler.yaml", "config-llm-scheduler-eppconfig-default-pd.yaml"} {
		preset := loadPresetConfig(t, name)
		preset.ObjectMeta = metav1.ObjectMeta{Name: preset.Name, Namespace: svc.Namespace}
		objects = append(objects, preset)
	}
	return svc, &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()}
}
