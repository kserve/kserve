/*
Copyright 2023 The KServe Authors.

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

package utils

import (
	"fmt"
	"strconv"
	"strings"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ReplacePlaceholders Replace placeholders in runtime container by values from inferenceservice metadata
func ReplacePlaceholders(container *corev1.Container, meta metav1.ObjectMeta) error {
	// First we normalize container to an unstructured map
	containerMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(container)
	if err != nil {
		return err
	}

	repl := &PlaceHolderReplacer{}
	err = repl.Replace(containerMap, meta)
	if err != nil {
		return err
	}

	return runtime.DefaultUnstructuredConverter.FromUnstructured(containerMap, container)
}

type PlaceHolderReplacer struct {
	meta       metav1.ObjectMeta
	parentPath []string
}

// replacerError is an error wrapper type for internal use only.
// can distinguish intentional panics from this package.
type replacerError struct{ error }

func (r *PlaceHolderReplacer) replace(v any) any {
	switch val := v.(type) {
	case string:
		hasTemplate := strings.Contains(val, "{{") && strings.Contains(val, "}}")
		if !hasTemplate {
			return val
		}

		tmpl, err := template.New("container-tmpl").Parse(val)
		if err != nil {
			panic(replacerError{err})
		}

		sb := strings.Builder{}
		if err = tmpl.Execute(&sb, r.meta); err != nil {
			panic(replacerError{err})
		}

		return sb.String()
	case map[string]any:
		return r.replaceMap(val)
	case []any:
		return r.replaceSlice(val)
	default:
		// Other types remain unchanged
		return val
	}
}

func (r *PlaceHolderReplacer) replaceMap(val map[string]any) any {
	pathLen := len(r.parentPath)
	for k, sub := range val {
		r.parentPath = append(r.parentPath, ".", k)
		val[k] = r.replace(sub)
		r.parentPath = r.parentPath[:pathLen]
	}
	return val
}

func (r *PlaceHolderReplacer) replaceSlice(val []any) any {
	pathLen := len(r.parentPath)
	for i, sub := range val {
		r.parentPath = append(r.parentPath, "[", strconv.Itoa(i), "]")
		val[i] = r.replace(sub)
		r.parentPath = r.parentPath[:pathLen]
	}
	return val
}

func (r *PlaceHolderReplacer) Replace(objMap map[string]any, meta metav1.ObjectMeta) (err error) {
	defer func() {
		if e := recover(); e != nil {
			if re, ok := e.(replacerError); ok {
				err = fmt.Errorf("failed to replace placeholder at %q: %w", strings.Join(r.parentPath, ""), re.error)
			} else {
				panic(e)
			}
		}
	}()

	r.meta = meta
	r.replace(objMap)
	return nil
}
