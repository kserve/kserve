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

// Package runtimeoptions resolves explicitly registered runtime options from a
// Kubernetes container without executing or fully interpreting its command.
package runtimeoptions

import (
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// Spec maps one runtime option to its canonical identity key.
type Spec struct {
	// Key is the canonical name returned by Resolve.
	Key string
	// Env lists direct environment variable names for this option.
	Env []string
	// Flags lists CLI aliases for this option.
	Flags []string
	// Boolean treats flag presence as true and supports --no-<flag>.
	Boolean bool
}

type optionSpec struct {
	spec Spec
	flag string
}

type specIndex struct {
	options []optionSpec
	env     map[string]Spec
}

// Resolve returns the last resolvable value for each registered option.
// Values are applied in Env, Args, and Command order.
func Resolve(container *corev1.Container, specs []Spec) map[string]string {
	result := map[string]string{}
	if container == nil {
		return result
	}

	index := indexSpecs(specs)
	resolveEnv(result, container.Env, index)
	parseOptionTokens(result, container.Args, index, false)
	resolveCommand(result, container.Command, container.Env, index)
	return result
}

func indexSpecs(specs []Spec) specIndex {
	index := specIndex{env: map[string]Spec{}}
	for _, spec := range specs {
		if spec.Key == "" {
			continue
		}
		for _, flag := range spec.Flags {
			if flag != "" {
				index.options = append(index.options, optionSpec{spec: spec, flag: flag})
			}
		}
		for _, name := range spec.Env {
			if name != "" {
				index.env[name] = spec
			}
		}
	}
	sort.SliceStable(index.options, func(left, right int) bool {
		return len(index.options[left].flag) > len(index.options[right].flag)
	})
	return index
}

func resolveEnv(result map[string]string, env []corev1.EnvVar, index specIndex) {
	for _, variable := range env {
		if variable.ValueFrom != nil {
			continue
		}
		spec, ok := index.env[variable.Name]
		if ok {
			result[spec.Key] = variable.Value
		}
	}
}

func parseOptionTokens(result map[string]string, tokens []string, index specIndex, skipUnresolved bool) {
	for position := 0; position < len(tokens); position++ {
		argument := tokens[position]
		if skipUnresolved && unresolved(argument) {
			continue
		}

		option, value, inline, found := matchOption(argument, index.options)
		if !found {
			continue
		}
		if option.spec.Boolean {
			if inline {
				result[option.spec.Key] = value
				continue
			}
			if position+1 < len(tokens) && isBooleanValue(tokens[position+1]) {
				result[option.spec.Key] = tokens[position+1]
				position++
				continue
			}
			result[option.spec.Key] = "true"
			continue
		}

		if inline {
			result[option.spec.Key] = value
			continue
		}
		if position+1 >= len(tokens) || (skipUnresolved && unresolved(tokens[position+1])) || strings.HasPrefix(tokens[position+1], "-") {
			continue
		}
		result[option.spec.Key] = tokens[position+1]
		position++
	}
}

func matchOption(argument string, options []optionSpec) (optionSpec, string, bool, bool) {
	for _, option := range options {
		if option.spec.Boolean && isBooleanNegationForm(argument, option.flag) {
			return option, "false", true, true
		}
		if argument == option.flag {
			return option, "", false, true
		}
		prefix := option.flag + "="
		if strings.HasPrefix(argument, prefix) {
			return option, strings.TrimPrefix(argument, prefix), true, true
		}
	}
	return optionSpec{}, "", false, false
}

func isBooleanNegationForm(argument, flag string) bool {
	return strings.HasPrefix(flag, "--") && argument == "--no-"+strings.TrimPrefix(flag, "--")
}

func isBooleanValue(value string) bool {
	switch value {
	case "true", "false", "1", "0":
		return true
	default:
		return false
	}
}

func unresolved(value string) bool {
	return strings.Contains(value, "$") || strings.Contains(value, "{{") || strings.Contains(value, "`")
}
