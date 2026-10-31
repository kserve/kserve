# Copyright 2026 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import importlib.util
import json
from pathlib import Path

import pytest


SCRIPT_PATH = Path(__file__).resolve().parents[1] / "generate-versions-from-gomod.py"
SPEC = importlib.util.spec_from_file_location(
    "generate_versions_from_gomod", SCRIPT_PATH
)
GENERATOR = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(GENERATOR)


@pytest.mark.parametrize(
    ("existing_repositories", "expected_commands"),
    [
        (
            [{"name": "istio", "url": "https://old.example/charts"}],
            [
                "helm repo add --force-update istio https://blob.istio.io/istio-release/charts"
            ],
        ),
        (
            [{"name": "istio", "url": "https://blob.istio.io/istio-release/charts"}],
            [],
        ),
        ([], ["helm repo add istio https://blob.istio.io/istio-release/charts"]),
    ],
)
def test_ensure_helm_repo_updates_only_when_needed(
    monkeypatch, existing_repositories, expected_commands
):
    commands = []

    def fake_run(command):
        if command == "helm repo list -o json || echo '[]'":
            return json.dumps(existing_repositories)
        commands.append(command)
        return ""

    monkeypatch.setattr(GENERATOR, "run", fake_run)

    GENERATOR.ensure_helm_repo("istio", "https://blob.istio.io/istio-release/charts")

    assert commands == expected_commands
