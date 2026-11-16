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

package testing

import (
	"bytes"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

func TestControllerLogFormatHelmRender(t *testing.T) {
	helm, err := exec.LookPath("helm")
	require.NoError(t, err, "helm is required to render charts")

	root := ProjectRoot()

	t.Run("default is zap", func(t *testing.T) {
		manager := renderChart(t, helm, root, "charts/kserve-resources")
		requireLogFormat(t, manager, "Deployment", "kserve-controller-manager", "--log-format=zap")

		localmodel := renderChart(t, helm, root, "charts/kserve-localmodel-resources")
		requireLogFormat(t, localmodel, "Deployment", "kserve-localmodel-controller-manager", "--log-format=zap")
		requireLogFormat(t, localmodel, "DaemonSet", "kserve-localmodelnode-agent", "--log-format=zap")
	})

	t.Run("otel-json when configured", func(t *testing.T) {
		manager := renderChart(t, helm, root, "charts/kserve-resources", "kserve.controller.logFormat=otel-json")
		requireLogFormat(t, manager, "Deployment", "kserve-controller-manager", "--log-format=otel-json")

		localmodel := renderChart(t, helm, root, "charts/kserve-localmodel-resources", "kserve.localmodel.controller.logFormat=otel-json")
		requireLogFormat(t, localmodel, "Deployment", "kserve-localmodel-controller-manager", "--log-format=otel-json")
		requireLogFormat(t, localmodel, "DaemonSet", "kserve-localmodelnode-agent", "--log-format=zap")

		node := renderChart(t, helm, root, "charts/kserve-localmodel-resources", "kserve.localmodelnode.controller.logFormat=otel-json")
		requireLogFormat(t, node, "Deployment", "kserve-localmodel-controller-manager", "--log-format=zap")
		requireLogFormat(t, node, "DaemonSet", "kserve-localmodelnode-agent", "--log-format=otel-json")
	})
}

func renderChart(t *testing.T, helm, root, chart string, sets ...string) []byte {
	t.Helper()

	args := []string{"template", "kserve", chart}
	for _, set := range sets {
		args = append(args, "--set", set)
	}

	cmd := exec.Command(helm, args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "helm template %s\n%s", chart, out)
	return out
}

func requireLogFormat(t *testing.T, manifest []byte, kind, name, want string) {
	t.Helper()

	args := managerArgs(t, manifest, kind, name)
	var got []string
	for _, arg := range args {
		if arg == "--log-format" || strings.HasPrefix(arg, "--log-format=") {
			got = append(got, arg)
		}
	}
	require.Equal(t, []string{want}, got, "%s %s log format args", kind, name)
}

func managerArgs(t *testing.T, manifest []byte, kind, name string) []string {
	t.Helper()

	decoder := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(manifest), 4096)
	for {
		obj := map[string]any{}
		err := decoder.Decode(&obj)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if len(obj) == 0 {
			continue
		}

		u := unstructured.Unstructured{Object: obj}
		if u.GetKind() != kind || u.GetName() != name {
			continue
		}

		containers, found, err := unstructured.NestedSlice(u.Object, "spec", "template", "spec", "containers")
		require.NoError(t, err)
		require.Truef(t, found, "%s %s has no containers", kind, name)
		for _, container := range containers {
			cm, ok := container.(map[string]any)
			require.True(t, ok)
			if cm["name"] != "manager" {
				continue
			}
			args, found, err := unstructured.NestedStringSlice(cm, "args")
			require.NoError(t, err)
			require.Truef(t, found, "%s %s manager container has no args", kind, name)
			return args
		}
		t.Fatalf("%s %s has no manager container", kind, name)
	}

	t.Fatalf("rendered manifests have no %s %s", kind, name)
	return nil
}
