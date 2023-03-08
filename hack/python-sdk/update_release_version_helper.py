#!/usr/bin/env python3

# Copyright 2023 The KServe Authors.
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

import tomlkit
import argparse

parser = argparse.ArgumentParser(description="Update release version in python toml files")
parser.add_argument("version", type=str, help="release version")
args, _ = parser.parse_known_args()

toml_files = [
    "python/kserve/pyproject.toml",
    "python/aiffairness/pyproject.toml",
    "python/aixexplainer/pyproject.toml",
    "python/alibiexplainer/pyproject.toml",
    "python/artexplainer/pyproject.toml",
    "python/custom_model/pyproject.toml",
    "python/custom_transformer/pyproject.toml",
    "python/lgbserver/pyproject.toml",
    "python/paddleserver/pyproject.toml",
    "python/pmmlserver/pyproject.toml",
    "python/sklearnserver/pyproject.toml",
    "python/xgbserver/pyproject.toml",
]

for toml_file in toml_files:
    with open(toml_file, "r") as file:
        toml_config = tomlkit.load(file)
        toml_config['tool']['poetry']['version'] = args.version

    with open(toml_file, "w") as file:
        tomlkit.dump(toml_config, file)
