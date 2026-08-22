/*
Copyright 2025 The KServe Authors.

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
	"context"
	"encoding/json"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/log"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// fieldPath holds the kyaml Lookup segments parsed from a single Kustomize-style path string.
// For example, "template.containers.[name=main].args" becomes
// ["template", "containers", "[name=main]", "args"].
type fieldPath = []string

// annotatedSpec pairs a spec with its source resource's metadata annotations,
// so the merge chain can read per-config directives like merge-append-fields.
type annotatedSpec struct {
	spec        v1alpha2.LLMInferenceServiceSpec
	annotations map[string]string
}

// ---------------------------------------------------------------------------
// Top-level merge orchestration
// ---------------------------------------------------------------------------

// mergeAnnotatedSpecs iterates through an ordered list of annotated specs,
// pairwise merging each into the accumulated result. For each override spec
// it parses the merge-append-fields annotation and concatenates the declared
// fields instead of replacing them.
func mergeAnnotatedSpecs(ctx context.Context, specs []annotatedSpec) (v1alpha2.LLMInferenceServiceSpec, error) {
	if len(specs) == 0 {
		return v1alpha2.LLMInferenceServiceSpec{}, nil
	}

	logger := log.FromContext(ctx)
	out := specs[0].spec

	for i := 1; i < len(specs); i++ {
		override := specs[i]
		appendPaths, err := v1alpha2.ParseMergeAppendFieldPaths(override.annotations)
		if err != nil {
			logger.V(1).Info("ignoring invalid merge-append-fields paths",
				"error", err,
				"configIndex", i,
			)
		}

		out, err = mergeSpecsWithAppend(ctx, out, override.spec, appendPaths)
		if err != nil {
			return v1alpha2.LLMInferenceServiceSpec{}, fmt.Errorf("failed to merge specs: %w", err)
		}
	}
	return out, nil
}

// mergeSpecsWithAppend performs a standard strategic merge of base and override,
// then concatenates the array fields listed in appendPaths instead of replacing
// them. When appendPaths is empty, it delegates directly to mergeSpecs with no
// additional overhead.
func mergeSpecsWithAppend(ctx context.Context, base, override v1alpha2.LLMInferenceServiceSpec, appendPaths []fieldPath) (v1alpha2.LLMInferenceServiceSpec, error) {
	if len(appendPaths) == 0 {
		return mergeSpecs(ctx, base, override)
	}

	baseJSON, err := json.Marshal(base)
	if err != nil {
		return v1alpha2.LLMInferenceServiceSpec{}, fmt.Errorf("marshal base for append: %w", err)
	}
	overrideJSON, err := json.Marshal(override)
	if err != nil {
		return v1alpha2.LLMInferenceServiceSpec{}, fmt.Errorf("marshal override for append: %w", err)
	}

	merged, err := mergeSpecs(ctx, base, override)
	if err != nil {
		return merged, err
	}

	mergedJSON, err := json.Marshal(merged)
	if err != nil {
		return merged, fmt.Errorf("marshal merged for append: %w", err)
	}

	fixedJSON, err := concatSequenceFields(baseJSON, overrideJSON, mergedJSON, appendPaths)
	if err != nil {
		return merged, fmt.Errorf("apply append paths: %w", err)
	}

	var result v1alpha2.LLMInferenceServiceSpec
	if err := json.Unmarshal(fixedJSON, &result); err != nil {
		return merged, fmt.Errorf("unmarshal after append: %w", err)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Sequence concatenation (post-merge fixup)
// ---------------------------------------------------------------------------

// concatSequenceFields fixes up a merged JSON document so that specific array
// fields contain the concatenation of base + override elements instead of
// the override-only values left by strategic merge.
//
// For each path, the function looks up the sequence in base, override, and
// merged. When both base and override contain non-empty sequences, the merged
// node's sequence is replaced with [base..., override...]. Paths that don't
// resolve in either side are silently skipped.
func concatSequenceFields(baseJSON, overrideJSON, mergedJSON []byte, paths []fieldPath) ([]byte, error) {
	if len(paths) == 0 {
		return mergedJSON, nil
	}

	mergedNode, err := kyaml.ConvertJSONToYamlNode(string(mergedJSON))
	if err != nil {
		return nil, fmt.Errorf("parse merged JSON for append: %w", err)
	}

	modified := false
	for _, segments := range paths {
		baseSeq, _ := lookupSequence(baseJSON, segments)
		overrideSeq, _ := lookupSequence(overrideJSON, segments)

		if len(baseSeq) == 0 || len(overrideSeq) == 0 {
			continue
		}

		target, lookupErr := mergedNode.Pipe(kyaml.Lookup(segments...))
		if lookupErr != nil || target == nil {
			continue
		}

		// Replace the merged sequence with base + override elements.
		target.YNode().Content = nil
		for _, elem := range baseSeq {
			if pipeErr := target.PipeE(kyaml.Append(elem.YNode())); pipeErr != nil {
				return nil, fmt.Errorf("append element at path %v: %w", segments, pipeErr)
			}
		}
		for _, elem := range overrideSeq {
			if pipeErr := target.PipeE(kyaml.Append(elem.YNode())); pipeErr != nil {
				return nil, fmt.Errorf("append element at path %v: %w", segments, pipeErr)
			}
		}
		modified = true
	}

	if !modified {
		return mergedJSON, nil
	}
	return mergedNode.MarshalJSON()
}

// ---------------------------------------------------------------------------
// kyaml lookup utilities
// ---------------------------------------------------------------------------

// lookupSequence parses JSON bytes into a kyaml RNode, navigates to the
// given field path, and returns the sequence elements at that location.
// Returns nil without error when the path does not exist or leads to a
// non-sequence node.
func lookupSequence(jsonBytes []byte, segments fieldPath) ([]*kyaml.RNode, error) {
	node, err := kyaml.ConvertJSONToYamlNode(string(jsonBytes))
	if err != nil {
		return nil, err
	}
	target, err := node.Pipe(kyaml.Lookup(segments...))
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, nil
	}
	return target.Elements()
}
