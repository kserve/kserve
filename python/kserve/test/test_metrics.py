# Copyright 2021 The KServe Authors.
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

# coding: utf-8

import unittest
from kserve.metrics import PRE_HIST_TIME, PROM_KEYS, get_labels
import os


def create_labels(value):
    expected_labels = {}
    for env_var, label in PROM_KEYS.items():
        # set the env vars for get_labels to read from
        os.environ[env_var] = value
        # create the expected labels to compare the result with
        expected_labels[label] = value
    return expected_labels


class TestGetLabels(unittest.TestCase):
    def setUp(self):
        pass

    def tearDown(self):
        pass

    def test_get_labels(self):
        expected_labels = create_labels(value="something")
        result_labels = get_labels()
        assert PRE_HIST_TIME.labels(**result_labels) == PRE_HIST_TIME.labels(**expected_labels)

    def test_get_labels_empty(self):
        expected_labels = create_labels(value="")
        result_labels = get_labels()
        assert PRE_HIST_TIME.labels(**result_labels) == PRE_HIST_TIME.labels(**expected_labels)
