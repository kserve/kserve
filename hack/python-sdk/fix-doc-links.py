#!/usr/bin/env python3

# Copyright 2026 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Resolve references to model pages omitted by OpenAPI Generator 4.3.1."""

import json
import os
from pathlib import Path
import re
import subprocess


REFERENCES = {
    "V1Condition": "https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#Condition",
    "ResourceQuantity": "https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource#Quantity",
    "ResourceInfDecAmount": "https://github.com/kubernetes/apimachinery/blob/master/pkg/api/resource/amount.go",
    "ResourceInt64Amount": "https://github.com/kubernetes/apimachinery/blob/master/pkg/api/resource/amount.go",
    "K8sIoApiAppsV1DeploymentStrategy": "https://pkg.go.dev/k8s.io/api/apps/v1#DeploymentStrategy",
    "K8sIoApiAutoscalingV2HorizontalPodAutoscalerBehavior": "https://pkg.go.dev/k8s.io/api/autoscaling/v2#HorizontalPodAutoscalerBehavior",
    "GithubComKedacoreKedaV2ApisKedaV1alpha1ScaleTriggers": "https://pkg.go.dev/github.com/kedacore/keda/v2/apis/keda/v1alpha1#ScaleTriggers",
    "KnativeDevServingPkgApisServingV1TrafficTarget": "https://pkg.go.dev/knative.dev/serving/pkg/apis/serving/v1#TrafficTarget",
}
CORE_TYPES = (
    "ContainerResizePolicy",
    "ContainerRestartRule",
    "PersistentVolumeSpec",
    "PersistentVolumeClaimSpec",
    "PodResourceClaim",
    "PodSchedulingGroup",
    "PodSchedulingGate",
    "EvictionResponder",
    "PodOS",
    "PersistentVolume",
    "PersistentVolumeClaim",
)


def module_versions(root):
    if not (root / "go.mod").exists():
        return {}
    output = subprocess.check_output(
        [
            "go",
            "list",
            "-m",
            "-json",
            "k8s.io/api",
            "k8s.io/apimachinery",
            "github.com/kedacore/keda/v2",
            "knative.dev/serving",
        ],
        cwd=root,
        text=True,
    )
    versions = {}
    decoder = json.JSONDecoder()
    while output.strip():
        module, end = decoder.raw_decode(output.lstrip())
        versions[module["Path"]] = module["Version"]
        output = output.lstrip()[end:]
    return versions


def pin_reference(url, versions):
    for module, version in sorted(versions.items(), key=lambda pair: -len(pair[0])):
        prefix = "https://pkg.go.dev/" + module
        if url.startswith(prefix + "/") or url.startswith(prefix + "#"):
            return prefix + "@" + version + url[len(prefix) :]
    if (
        url.startswith("https://github.com/kubernetes/apimachinery/blob/master/")
        and "k8s.io/apimachinery" in versions
    ):
        return url.replace(
            "/blob/master/", "/blob/" + versions["k8s.io/apimachinery"] + "/", 1
        )
    return url


def rewrite_docs(root, imports, *, versions=None):
    docs = root / "python/kserve/docs"
    references = dict(REFERENCES)
    references.update(
        {
            "V1" + name: "https://pkg.go.dev/k8s.io/api/core/v1#" + name
            for name in CORE_TYPES
        }
    )
    for name in imports:
        references[name] = (
            "https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/"
            + name
            + ".md"
        )
    versions = module_versions(root) if versions is None else versions
    references = {
        name: pin_reference(url, versions) for name, url in references.items()
    }
    for source in sorted((root / "pkg/apis/serving").glob("*/*.go")):
        if source.name.endswith("_test.go") or source.name.startswith("zz_"):
            continue
        for text in source.read_text().splitlines():
            if match := re.match(r"type ([A-Z]\w*)\b", text):
                name = source.parent.name.capitalize() + match[1]
                references[name] = os.path.relpath(source, docs)
    changes = {}
    for document in sorted(docs.glob("*.md")):

        def replace(match, document=document):
            filename = match[1]
            if (docs / filename).is_file():
                return match[0]
            name = filename[:-3]
            destination = references.get(name)
            if destination is None:
                raise ValueError(
                    f"{document}: no documentation target for {name}; extend CORE_TYPES or REFERENCES and regenerate"
                )
            return "(" + destination + ")"

        lines = []
        for original in document.read_text().splitlines():
            line = original.replace("[**object**](.md)", "**object**")
            line = re.sub(r"\(([^()/\s]+\.md)\)", replace, line)
            lines.append(line)
        changes[document] = "\n".join(lines) + "\n"
    for document, text in changes.items():
        document.write_text(text)


if __name__ == "__main__":
    root = Path(__file__).resolve().parents[2]
    config = json.loads((root / "hack/python-sdk/swagger_config.json").read_text())
    rewrite_docs(root, config["importMappings"])
