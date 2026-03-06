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

// Package main provides a tool to test the config merge logic.
// It reads a list of config files (LLMInferenceServiceConfig or LLMInferenceService)
// and merges them together to inspect the final spec.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"sigs.k8s.io/yaml"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

func main() {
	outputYAML := flag.Bool("yaml", false, "Output in YAML format (default is JSON)")
	includeBaseConfigs := flag.Bool("include-base-configs", true, "Automatically include base configs from config/llmisvcconfig/")
	baseConfigsDir := flag.String("base-configs-dir", "config/llmisvcconfig", "Directory containing base config-llm-*.yaml files")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <config1.yaml> [config2.yaml] [config3.yaml] ...\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nMerges multiple LLMInferenceServiceConfig or LLMInferenceService specs in order.\n")
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		fmt.Fprintf(os.Stderr, "  -base-configs-dir string\n")
		fmt.Fprintf(os.Stderr, "        Directory containing base config-llm-*.yaml files (default \"config/llmisvcconfig\")\n")
		fmt.Fprintf(os.Stderr, "  -include-base-configs\n")
		fmt.Fprintf(os.Stderr, "        Automatically include base configs from config/llmisvcconfig/ (default true)\n")
		fmt.Fprintf(os.Stderr, "  -yaml\n")
		fmt.Fprintf(os.Stderr, "        Output in YAML format (default is JSON)\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Auto-include base configs (default):\n")
		fmt.Fprintf(os.Stderr, "  %s llmisvc_config.yaml inference.yaml\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\n  # Disable auto-include:\n")
		fmt.Fprintf(os.Stderr, "  %s -include-base-configs=false config1.yaml config2.yaml\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\n  # Output as YAML:\n")
		fmt.Fprintf(os.Stderr, "  %s -yaml llmisvc_config.yaml inference.yaml\n", os.Args[0])
	}
	flag.Parse()

	configFiles := flag.Args()
	if len(configFiles) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	// Auto-discover and prepend base configs if enabled
	if *includeBaseConfigs {
		baseConfigs, err := discoverBaseConfigs(*baseConfigsDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to discover base configs: %v\n", err)
			fmt.Fprintf(os.Stderr, "Continuing without auto-discovered base configs...\n\n")
		} else if len(baseConfigs) > 0 {
			fmt.Fprintf(os.Stderr, "Auto-discovered %d base config(s) from %s\n", len(baseConfigs), *baseConfigsDir)
			configFiles = append(baseConfigs, configFiles...)
		}
	}
	var specs []v1alpha2.LLMInferenceServiceSpec

	// Read and parse all config files
	for _, file := range configFiles {
		fmt.Fprintf(os.Stderr, "Reading: %s\n", file)

		data, err := os.ReadFile(file) // #nosec G304 -- Tool intentionally reads user-specified config files
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file %s: %v\n", file, err)
			os.Exit(1)
		}

		// Try to parse as LLMInferenceServiceConfig first
		var config v1alpha2.LLMInferenceServiceConfig
		if err := yaml.Unmarshal(data, &config); err == nil && config.Kind == "LLMInferenceServiceConfig" {
			fmt.Fprintf(os.Stderr, "  -> Parsed as LLMInferenceServiceConfig\n")
			specs = append(specs, config.Spec)
			continue
		}

		// Try to parse as LLMInferenceService
		var svc v1alpha2.LLMInferenceService
		if err := yaml.Unmarshal(data, &svc); err == nil && svc.Kind == "LLMInferenceService" {
			fmt.Fprintf(os.Stderr, "  -> Parsed as LLMInferenceService\n")
			specs = append(specs, svc.Spec)
			continue
		}

		fmt.Fprintf(os.Stderr, "Error: Could not parse %s as LLMInferenceServiceConfig or LLMInferenceService\n", file)
		os.Exit(1)
	}

	if len(specs) == 0 {
		fmt.Fprintf(os.Stderr, "Error: No specs to merge\n")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "\nMerging %d specs...\n\n", len(specs))

	// Merge all the specs
	ctx := context.Background()
	merged, err := llmisvc.MergeSpecs(ctx, specs...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error merging specs: %v\n", err)
		os.Exit(1)
	}

	// Output the merged result
	if *outputYAML {
		mergedYAML, err := yaml.Marshal(merged)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error converting to YAML: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(mergedYAML))
	} else {
		mergedJSON, err := json.MarshalIndent(merged, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error converting to JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(mergedJSON))
	}
}

// discoverBaseConfigs finds all config-llm-*.yaml files in the specified directory
// and returns them in sorted order for consistent merging.
func discoverBaseConfigs(dir string) ([]string, error) {
	pattern := filepath.Join(dir, "config-llm-*.yaml")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	// Sort to ensure consistent order
	sort.Strings(matches)

	return matches, nil
}
