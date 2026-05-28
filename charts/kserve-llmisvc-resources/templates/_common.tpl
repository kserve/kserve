{{/*
Common resource rendering helpers for kserve charts.
These helpers encapsulate the logic for loading, patching, and rendering common resources.
*/}}

{{/*
Render a simple resource by loading a base file and optionally replacing namespace.

Parameters (dict):
  - baseFile: string - Path to the base resource file (e.g., "files/common/certmanager.yaml")
  - replaceNamespace: boolean - Whether to replace namespace in the resource
  - context: . - The root context for accessing .Release, .Files, etc.

Example usage:
{{- include "kserve-common.renderSimpleResource" (dict
  "baseFile" "files/common/certmanager.yaml"
  "replaceNamespace" true
  "context" .) -}}
*/}}
{{- define "kserve-common.renderSimpleResource" -}}
{{- $baseContent := .context.Files.Get .baseFile -}}
{{- if .replaceNamespace -}}
{{- $baseContent = include "kserve-common.replaceNamespace" (list $baseContent .context.Release.Namespace) -}}
{{- end -}}
---
{{ $baseContent }}
{{- end -}}

{{/*
Render a patched resource by loading base and patch files, optionally replacing namespace,
then merging them using deep merge.

Parameters (dict):
  - baseFile: string - Path to the base resource file
  - patchFile: string - Path to the patch resource file
  - replaceNamespace: boolean - Whether to replace namespace in the base resource before merging
  - context: . - The root context for accessing .Release, .Files, .Values, etc.

Example usage:
{{- include "kserve-common.renderPatchedResource" (dict
  "baseFile" "files/common/configmap.yaml"
  "patchFile" "files/common/configmap-patch.yaml"
  "replaceNamespace" true
  "context" .) -}}
*/}}
{{- define "kserve-common.renderPatchedResource" -}}
{{- /* 1. Load base resource file */ -}}
{{- $baseContent := .context.Files.Get .baseFile | toString -}}

{{- /* 2. Optionally replace namespace in base content */ -}}
{{- if .replaceNamespace -}}
{{- $baseContent = include "kserve-common.replaceNamespace" (list $baseContent .context.Release.Namespace) -}}
{{- end -}}

{{- /* 3. Load and render patch file with tpl */ -}}
{{- $patchContent := .context.Files.Get .patchFile | toString -}}
{{- $patchRendered := tpl $patchContent .context -}}

{{- /* 4. Parse base and patch as YAML */ -}}
{{- $base := fromYaml $baseContent -}}
{{- $patch := fromYaml $patchRendered -}}

{{- /* 5. Merge patch into base using deep merge */ -}}
{{- if and $base $patch -}}
{{- $merged := include "kserve-common.deepMerge" (list $base $patch) | fromYaml -}}
---
{{ toYaml $merged }}
{{- else if $base -}}
---
{{ toYaml $base }}
{{- end -}}
{{- end -}}
