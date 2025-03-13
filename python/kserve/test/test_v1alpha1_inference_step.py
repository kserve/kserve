# Copyright 2022 The KServe Authors.
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

"""
KServe

Python SDK for KServe  # noqa: E501

The version of the OpenAPI document: v0.1
Generated by: https://openapi-generator.tech
"""


from __future__ import absolute_import

import unittest
import datetime

import kserve
from kserve.models.v1alpha1_inference_step import V1alpha1InferenceStep  # noqa: E501
from kserve.rest import ApiException


class TestV1alpha1InferenceStep(unittest.TestCase):
    """V1alpha1InferenceStep unit test stubs"""

    def setUp(self):
        pass

    def tearDown(self):
        pass

    def make_instance(self, include_optional):
        """Test V1alpha1InferenceStep
        include_option is a boolean, when False only required
        params are included, when True both required and
        optional params are included"""
        # model = kserve.models.v1alpha1_inference_step.V1alpha1InferenceStep()  # noqa: E501
        if include_optional:
            return V1alpha1InferenceStep(
                condition="0",
                data="0",
                name="0",
                node_name="0",
                service_name="0",
                service_url="0",
                weight=56,
            )
        else:
            return V1alpha1InferenceStep()

    def testV1alpha1InferenceStep(self):
        """Test V1alpha1InferenceStep"""
        inst_req_only = self.make_instance(include_optional=False)
        inst_req_and_optional = self.make_instance(include_optional=True)


if __name__ == "__main__":
    unittest.main()
