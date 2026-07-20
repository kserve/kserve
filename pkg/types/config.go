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

package types

// OCI model mode constants for OciModelMode field.
const (
	OciModelModeModelcar = "modelcar"
	OciModelModeNative   = "native"
	OciModelModeFetch    = "fetch"
)

type StorageInitializerConfig struct {
	Image                   string `json:"image"`
	CpuRequest              string `json:"cpuRequest"`
	CpuLimit                string `json:"cpuLimit"`
	CpuModelcar             string `json:"cpuModelcar"`
	MemoryRequest           string `json:"memoryRequest"`
	MemoryLimit             string `json:"memoryLimit"`
	CaBundleConfigMapName   string `json:"caBundleConfigMapName"`
	CaBundleVolumeMountPath string `json:"caBundleVolumeMountPath"`
	MemoryModelcar          string `json:"memoryModelcar"`
	EnableOciImageSource    bool   `json:"enableModelcar"`
	UidModelcar             *int64 `json:"uidModelcar"`
	// EnableOciModelSupport enables any OCI-backed model storage path.
	// Backcompat: EnableOciImageSource (enableModelcar) remains functional; this is the newer switch.
	EnableOciModelSupport bool `json:"enableOciModelSupport"`
	// OciModelMode selects the materialization strategy for oci:// and oci+native:// URIs.
	// Valid values: "modelcar" (default), "native", "fetch". Empty resolves to "modelcar".
	OciModelMode string `json:"ociModelMode"`
	// OciInsecureRegistry opts the oci+fetch:// storage-initializer path out of TLS
	// verification entirely (plain HTTP or self-signed certs with no distributable CA
	// bundle). Defaults to false (secure/verified HTTPS) -- this must be an explicit
	// opt-in, never inferred from the registry host. Wired to the init container via
	// the KSERVE_OCI_INSECURE_REGISTRY env var (see ConfigureOciFetchToContainer).
	OciInsecureRegistry bool `json:"ociInsecureRegistry"`
}

// ResolveOciModelMode returns the effective OCI model mode for the given config.
// Priority: OciModelMode (explicit) > EnableOciImageSource/EnableOciModelSupport (backcompat) > "" (disabled).
func ResolveOciModelMode(cfg *StorageInitializerConfig) string {
	if cfg.OciModelMode != "" {
		return cfg.OciModelMode
	}
	if cfg.EnableOciImageSource || cfg.EnableOciModelSupport {
		return OciModelModeModelcar
	}
	return ""
}
