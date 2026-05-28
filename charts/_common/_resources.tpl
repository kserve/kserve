{{/*
Render multiple Kubernetes resources with patches applied.

This helper loads a base multi-document YAML file, applies namespace replacement,
replaces cert-manager annotations, loads and merges multiple patch files using
glob patterns, and outputs all merged resources sorted by kind/name.

Parameters (dict):
  - baseFile: string - Path to the base resources file (multi-document YAML)
  - patchGlob: string - Glob pattern for patch files (e.g., "files/kserve/*-patch.yaml")
  - certName: string - The cert name portion after namespace (e.g., "serving-cert", "llmisvc-serving-cert")
  - context: . - The root context for accessing .Release, .Files, etc.

Example usage:
{{- include "kserve-common.renderMultiResourceWithPatches" (dict
  "baseFile" "files/kserve/resources.yaml"
  "patchGlob" "files/kserve/*-patch.yaml"
  "certName" "serving-cert"
  "context" .) -}}
*/}}
{{- define "kserve-common.renderMultiResourceWithPatches" -}}
{{- /* 1. Load base resources file and replace namespace */ -}}
{{- $baseContent := .context.Files.Get .baseFile -}}
{{- $baseContent = include "kserve-common.replaceNamespace" (list $baseContent .context.Release.Namespace) -}}

{{- /* 1.1. Replace cert-manager annotation with correct namespace */ -}}
{{- $pattern := printf "cert-manager.io/inject-ca-from: kserve/%s" .certName -}}
{{- $replacement := printf "cert-manager.io/inject-ca-from: %s/%s" .context.Release.Namespace .certName -}}
{{- $baseContent = $baseContent | replace $pattern $replacement -}}

{{- /* 2. Load and render all patch files */ -}}
{{- $allPatchContent := "" -}}
{{- range $path, $_ := .context.Files.Glob .patchGlob -}}
  {{- $patchContent := $.context.Files.Get $path | toString -}}
  {{- if $patchContent -}}
    {{- $allPatchContent = printf "%s\n---\n%s" $allPatchContent $patchContent -}}
  {{- end -}}
{{- end -}}

{{- /* 3. Render patch content with template variables */ -}}
{{- $patchRendered := tpl $allPatchContent .context -}}

{{- /* 4. Parse base resources into a map by kind/name */ -}}
{{- $baseMap := dict -}}
{{- range $obj := splitList "---" $baseContent -}}
  {{- $trimmed := trim $obj -}}
  {{- if $trimmed -}}
    {{- $parsed := fromYaml $trimmed -}}
    {{- if and $parsed $parsed.kind $parsed.metadata $parsed.metadata.name -}}
      {{- $key := printf "%s/%s" $parsed.kind $parsed.metadata.name -}}
      {{- $_ := set $baseMap $key $parsed -}}
    {{- end -}}
  {{- end -}}
{{- end -}}

{{- /* 5. Parse rendered patch resources */ -}}
{{- $patchMap := dict -}}
{{- range $obj := splitList "---" $patchRendered -}}
  {{- $trimmed := trim $obj -}}
  {{- if $trimmed -}}
    {{- $parsed := fromYaml $trimmed -}}
    {{- if and $parsed $parsed.kind $parsed.metadata $parsed.metadata.name -}}
      {{- $key := printf "%s/%s" $parsed.kind $parsed.metadata.name -}}
      {{- $_ := set $patchMap $key $parsed -}}
    {{- end -}}
  {{- end -}}
{{- end -}}

{{- /* 6. Merge patch into base using deep merge */ -}}
{{- range $key, $patch := $patchMap -}}
  {{- if hasKey $baseMap $key -}}
    {{- $base := get $baseMap $key -}}
    {{- $merged := include "kserve-common.deepMerge" (list $base $patch) | fromYaml -}}
    {{- $_ := set $baseMap $key $merged -}}
  {{- else -}}
    {{- /* New resource from patch */ -}}
    {{- $_ := set $baseMap $key $patch -}}
  {{- end -}}
{{- end -}}

{{- /* 7. Output all merged resources */ -}}
{{- range $key := keys $baseMap | sortAlpha }}
{{- $resource := index $baseMap $key }}
---
{{ toYaml $resource }}
{{ end -}}
{{- end -}}
