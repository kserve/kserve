// +build e2e

/*
Copyright 2019 The Knative Authors

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

package conformance

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/knative/serving/test"
	"k8s.io/apimachinery/pkg/api/resource"

	corev1 "k8s.io/api/core/v1"
)

const (
	cpuLimit    = 1     // CPU
	memoryLimit = 128   // MB
	cpuRequest  = 0.125 // CPU
)

func toMilliValue(value float64) string {
	return fmt.Sprintf("%dm", int(value*1000))
}

// TestMustHaveCgroupConfigured verifies that the Linux cgroups are configured based on the specified
// resource limits and requests as delared by "MUST" in the runtime-contract.
func TestMustHaveCgroupConfigured(t *testing.T) {
	t.Parallel()
	clients := setup(t)

	resources := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(toMilliValue(cpuLimit)),
			corev1.ResourceMemory: resource.MustParse(strconv.Itoa(memoryLimit) + "M"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse(toMilliValue(cpuRequest)),
		},
	}

	// Cgroup settings are based on the CPU and Memory Limits as well as CPU Reuqests
	// https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/
	expectedCgroups := map[string]int{
		"/sys/fs/cgroup/memory/memory.limit_in_bytes": memoryLimit * 1000000, // 128 MB
		"/sys/fs/cgroup/cpu/cpu.cfs_period_us":        100000,                // 100ms (100,000us) default
		"/sys/fs/cgroup/cpu/cpu.cfs_quota_us":         cpuLimit * 1000 * 100, // 1000 millicore * 100
		"/sys/fs/cgroup/cpu/cpu.shares":               cpuRequest * 1024}     // CPURequests * 1024

	ri, err := fetchRuntimeInfo(t, clients, &test.Options{ContainerResources: resources})
	if err != nil {
		t.Fatalf("Error fetching runtime info: %v", err)
	}

	cgroups := ri.Host.Cgroups

	for _, cgroup := range cgroups {
		if cgroup.Error != "" {
			t.Errorf("Error getting cgroup information: %v", cgroup.Error)
			continue
		}
		if _, ok := expectedCgroups[cgroup.Name]; !ok {
			// Service returned a value we don't test
			t.Logf("%v cgroup returned, but not validated", cgroup.Name)
			continue
		}
		if got, want := *cgroup.Value, expectedCgroups[cgroup.Name]; got != want {
			t.Errorf("%s = %d, want: %d", cgroup.Name, *cgroup.Value, expectedCgroups[cgroup.Name])
		}
	}
}

// TestShouldHaveCgroupReadOnly verifies that the Linux cgroups are mounted read-only within the
// container.
func TestShouldHaveCgroupReadOnly(t *testing.T) {
	t.Parallel()
	clients := setup(t)
	ri, err := fetchRuntimeInfo(t, clients, &test.Options{})
	if err != nil {
		t.Fatalf("Error fetching runtime info: %v", err)
	}

	cgroups := ri.Host.Cgroups

	for _, cgroup := range cgroups {
		if cgroup.Error != "" {
			t.Errorf("Error getting cgroup information: %v", cgroup.Error)
			continue
		}
		if got, want := *cgroup.ReadOnly, true; got != want {
			t.Errorf("For cgroup %s cgroup.ReadOnly = %v, want: %v", cgroup.Name, *cgroup.ReadOnly, want)
		}
	}

}
