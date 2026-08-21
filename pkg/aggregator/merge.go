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

package aggregator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// openAIModelList is the OpenAI-compatible GET /v1/models response shape.
type openAIModelList struct {
	Object string        `json:"object"`
	Data   []openAIModel `json:"data"`
}

type openAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created,omitempty"`
	OwnedBy string `json:"owned_by,omitempty"`
	// Source identifies which LLMInferenceService contributed this model entry.
	Source string `json:"source,omitempty"`
}

// MergeModels unions OpenAI model list entries across backends.
// Partial failures are omitted; if every backend fails, returns 502.
func MergeModels(results []BackendResult) ([]byte, int, error) {
	seen := map[string]struct{}{}
	out := openAIModelList{Object: "list", Data: []openAIModel{}}
	successes := 0

	for _, r := range results {
		if !r.OK() {
			continue
		}
		var list openAIModelList
		if err := json.Unmarshal(r.Body, &list); err != nil {
			// HTTP 200 with a non-OpenAI body is not a usable /v1/models result.
			continue
		}
		successes++
		for _, m := range list.Data {
			key := m.ID + "|" + r.Backend.ID()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			if m.Object == "" {
				m.Object = "model"
			}
			m.Source = r.Backend.ID()
			out.Data = append(out.Data, m)
		}
	}

	if successes == 0 && len(results) > 0 {
		return []byte(`{"object":"list","data":[],"error":"all backends failed"}`), http.StatusBadGateway, nil
	}
	body, err := json.Marshal(out)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return body, http.StatusOK, nil
}

// MergeHealth is healthy only when every backend is healthy (and there is at least one).
func MergeHealth(results []BackendResult) ([]byte, int, error) {
	if len(results) == 0 {
		body, _ := json.Marshal(map[string]any{
			"status":  "unhealthy",
			"reason":  "no backends",
			"results": []any{},
		})
		return body, http.StatusServiceUnavailable, nil
	}

	type backendHealth struct {
		Backend string `json:"backend"`
		OK      bool   `json:"ok"`
		Status  int    `json:"status,omitempty"`
		Error   string `json:"error,omitempty"`
	}

	allOK := true
	details := make([]backendHealth, 0, len(results))
	for _, r := range results {
		item := backendHealth{Backend: r.Backend.ID(), OK: r.OK(), Status: r.StatusCode}
		if r.Err != nil {
			item.Error = r.Err.Error()
			allOK = false
		} else if !r.OK() {
			allOK = false
			item.Error = fmt.Sprintf("status %d", r.StatusCode)
		}
		details = append(details, item)
	}

	status := "healthy"
	code := http.StatusOK
	if !allOK {
		status = "unhealthy"
		code = http.StatusServiceUnavailable
	}
	body, err := json.Marshal(map[string]any{
		"status":  status,
		"results": details,
	})
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return body, code, nil
}

// MergeMetrics concatenates Prometheus text and injects a backend label on metric samples.
// Failed backends are omitted; if all fail, returns 502.
func MergeMetrics(results []BackendResult) ([]byte, int, error) {
	var b strings.Builder
	successes := 0
	for _, r := range results {
		if !r.OK() {
			continue
		}
		successes++
		fmt.Fprintf(&b, "# BACKEND %s\n", r.Backend.ID())
		labeled := injectPrometheusBackendLabel(string(r.Body), r.Backend.ID())
		b.WriteString(labeled)
		if !strings.HasSuffix(labeled, "\n") {
			b.WriteByte('\n')
		}
	}
	if successes == 0 && len(results) > 0 {
		return []byte("# all backends failed\n"), http.StatusBadGateway, nil
	}
	return []byte(b.String()), http.StatusOK, nil
}

// injectPrometheusBackendLabel adds backend="<id>" to each metric sample line.
func injectPrometheusBackendLabel(metrics, backendID string) string {
	label := fmt.Sprintf(`backend=%q`, backendID)
	lines := strings.Split(metrics, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		lines[i] = injectLabel(line, label)
	}
	return strings.Join(lines, "\n")
}

func injectLabel(sampleLine, label string) string {
	// metric{labels} value [timestamp]  OR  metric value
	brace := strings.IndexByte(sampleLine, '{')
	if brace >= 0 {
		closeBrace := strings.IndexByte(sampleLine[brace:], '}')
		if closeBrace < 0 {
			return sampleLine
		}
		closeBrace += brace
		inner := sampleLine[brace+1 : closeBrace]
		if inner == "" {
			return sampleLine[:brace+1] + label + sampleLine[closeBrace:]
		}
		return sampleLine[:brace+1] + inner + "," + label + sampleLine[closeBrace:]
	}
	// Insert labels before the first whitespace separating value.
	fields := strings.Fields(sampleLine)
	if len(fields) < 2 {
		return sampleLine
	}
	name := fields[0]
	rest := strings.TrimSpace(sampleLine[len(name):])
	return name + "{" + label + "} " + rest
}

// MergeLoad aggregates /load payloads keyed by backend ID.
// Failed backends are omitted; if all fail, returns 502.
func MergeLoad(results []BackendResult) ([]byte, int, error) {
	out := map[string]any{}
	successes := 0
	for _, r := range results {
		if !r.OK() {
			continue
		}
		successes++
		var payload any
		if err := json.Unmarshal(r.Body, &payload); err != nil {
			payload = map[string]any{"raw": string(r.Body)}
		}
		out[r.Backend.ID()] = payload
	}
	if successes == 0 && len(results) > 0 {
		body, _ := json.Marshal(map[string]any{"error": "all backends failed"})
		return body, http.StatusBadGateway, nil
	}
	body, err := json.Marshal(map[string]any{"object": "load", "backends": out})
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return body, http.StatusOK, nil
}

func defaultMergers() map[string]Merger {
	return map[string]Merger{
		PathModels:  MergeModels,
		PathHealth:  MergeHealth,
		PathMetrics: MergeMetrics,
		PathLoad:    MergeLoad,
	}
}
