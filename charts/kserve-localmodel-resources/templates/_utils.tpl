{{/*
Deep merge two dictionaries (recursive)

Usage: include "kserve-common.deepMerge" (list $base $patch) | fromYaml

Features:
- Merges dictionaries (maps) recursively
- Arrays with named elements (e.g., containers, env, volumeMounts) are merged by name
- Other arrays in $patch completely replace arrays in $base

Example:
  Base:    {a: {b: 1, c: 2}, containers: [{name: foo, x: 1}]}
  Patch:   {a: {b: 10}, containers: [{name: foo, y: 2}]}
  Result:  {a: {b: 10, c: 2}, containers: [{name: foo, x: 1, y: 2}]}
*/}}
{{- define "kserve-common.deepMerge" -}}
{{- $base := index . 0 -}}
{{- $patch := index . 1 -}}
{{- range $key, $value := $patch -}}
  {{- if hasKey $base $key -}}
    {{- $baseValue := get $base $key -}}
    {{- if and (kindIs "map" $value) (kindIs "map" $baseValue) -}}
      {{- /* Recursively merge nested maps */ -}}
      {{- $merged := include "kserve-common.deepMerge" (list $baseValue $value) | fromYaml -}}
      {{- $_ := set $base $key $merged -}}
    {{- else if and (kindIs "slice" $value) (kindIs "slice" $baseValue) -}}
      {{- /* Check if array elements have 'name' field for smart merge */ -}}
      {{- $canMergeByName := false -}}
      {{- if gt (len $value) 0 -}}
        {{- $firstElem := index $value 0 -}}
        {{- if and (kindIs "map" $firstElem) (hasKey $firstElem "name") -}}
          {{- $canMergeByName = true -}}
        {{- end -}}
      {{- end -}}
      {{- if $canMergeByName -}}
        {{- /* Merge arrays by name */ -}}
        {{- $mergedResult := include "kserve-common.mergeArrayByName" (list $baseValue $value) | fromYaml -}}
        {{- $_ := set $base $key (get $mergedResult "items") -}}
      {{- else -}}
        {{- /* Replace array completely */ -}}
        {{- $_ := set $base $key $value -}}
      {{- end -}}
    {{- else -}}
      {{- $_ := set $base $key $value -}}
    {{- end -}}
  {{- else -}}
    {{- $_ := set $base $key $value -}}
  {{- end -}}
{{- end -}}
{{ toYaml $base }}
{{- end }}

{{/*
Merge two arrays by matching 'name' field

Usage: include "kserve-common.mergeArrayByName" (list $baseArray $patchArray) | fromYaml

For each item in patch array:
- If matching name exists in base, merge them (recursively)
- Otherwise add as new item
Items in base without matching patch are preserved
*/}}
{{- define "kserve-common.mergeArrayByName" -}}
{{- $baseArray := index . 0 -}}
{{- $patchArray := index . 1 -}}
{{- $processedNames := dict -}}
{{- $result := dict "items" (list) -}}

{{- /* First pass: merge matching items from base with patches */ -}}
{{- range $baseItem := $baseArray -}}
  {{- if not (hasKey $baseItem "name") -}}
    {{- fail "mergeArrayByName: base array item missing 'name' field" -}}
  {{- end -}}
  {{- $name := $baseItem.name -}}
  {{- $matched := false -}}
  {{- range $patchItem := $patchArray -}}
    {{- if not (hasKey $patchItem "name") -}}
      {{- fail "mergeArrayByName: patch array item missing 'name' field" -}}
    {{- end -}}
    {{- if eq $patchItem.name $name -}}
      {{- /* Found matching item - merge them */ -}}
      {{- $merged := include "kserve-common.deepMerge" (list $baseItem $patchItem) | fromYaml -}}
      {{- $_ := set $result "items" (append (get $result "items") $merged) -}}
      {{- $_ := set $processedNames $name true -}}
      {{- $matched = true -}}
    {{- end -}}
  {{- end -}}
  {{- if not $matched -}}
    {{- /* No patch for this base item - keep as is */ -}}
    {{- $_ := set $result "items" (append (get $result "items") $baseItem) -}}
  {{- end -}}
{{- end -}}

{{- /* Second pass: add new items from patch that weren't in base */ -}}
{{- range $patchItem := $patchArray -}}
  {{- if not (hasKey $patchItem "name") -}}
    {{- fail "mergeArrayByName: patch array item missing 'name' field in second pass" -}}
  {{- end -}}
  {{- if not (hasKey $processedNames $patchItem.name) -}}
    {{- $_ := set $result "items" (append (get $result "items") $patchItem) -}}
  {{- end -}}
{{- end -}}

{{ toYaml $result }}
{{- end }}

{{/*
Safe namespace replacement - only replaces exact "namespace: kserve" pattern
*/}}
{{- define "kserve-common.replaceNamespace" -}}
{{- $content := index . 0 -}}
{{- $namespace := index . 1 -}}
{{- $pattern := "namespace: kserve\n" -}}
{{- $replacement := printf "namespace: %s\n" $namespace -}}
{{- $content | replace $pattern $replacement -}}
{{- end -}}
