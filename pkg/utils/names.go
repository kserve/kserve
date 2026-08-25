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

package utils

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// dns1123InvalidCharsRe matches characters invalid in DNS-1123 labels
// (Kubernetes volume and object names): anything but lowercase alphanumerics
// and hyphens.
var dns1123InvalidCharsRe = regexp.MustCompile(`[^a-z0-9-]`)

// sanitizeDNS1123Label transforms an arbitrary identifier into the DNS-1123
// label character set: lowercased, invalid characters replaced with hyphens,
// leading and trailing hyphens trimmed. The result may be empty and is not
// length-bounded; use SafeObjectName for a complete object name.
func sanitizeDNS1123Label(name string) string {
	sanitized := dns1123InvalidCharsRe.ReplaceAllString(strings.ToLower(name), "-")
	return strings.Trim(sanitized, "-")
}

// SafeObjectName makes a candidate string a valid Kubernetes object name:
// DNS-1123 label character set, at most 63 characters. Valid candidates pass
// through unchanged, so adopting this helper does not rename existing
// resources. Sanitized candidates carry a short hash of the original, keeping
// distinct candidates distinct ("a/b" and "a.b" both reduce to "a-b");
// over-long results are truncated in favor of the hash. Typically composed
// with kmeta.ChildName, e.g. SafeObjectName(kmeta.ChildName(prefix, name)).
func SafeObjectName(candidate string) string {
	sanitized := sanitizeDNS1123Label(candidate)
	if sanitized == candidate && len(candidate) <= validation.DNS1123LabelMaxLength {
		return candidate
	}

	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(candidate)))[:8]
	budget := validation.DNS1123LabelMaxLength - len(digest) - 1
	if len(sanitized) > budget {
		sanitized = strings.TrimRight(sanitized[:budget], "-")
	}
	if sanitized == "" {
		return digest
	}
	return sanitized + "-" + digest
}
