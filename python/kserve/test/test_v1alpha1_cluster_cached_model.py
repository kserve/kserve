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

# coding: utf-8

"""
    KServe

    Python SDK for KServe  # noqa: E501

    The version of the OpenAPI document: v0.1
    Generated by: https://openapi-generator.tech
"""


from __future__ import absolute_import

import datetime
import unittest

from kserve.models.v1alpha1_cluster_cached_model import (  # noqa: E501
    V1alpha1ClusterCachedModel,
)
from kserve.rest import ApiException

import kserve


class TestV1alpha1ClusterCachedModel(unittest.TestCase):
    """V1alpha1ClusterCachedModel unit test stubs"""

    def setUp(self):
        pass

    def tearDown(self):
        pass

    def make_instance(self, include_optional):
        """Test V1alpha1ClusterCachedModel
        include_option is a boolean, when False only required
        params are included, when True both required and
        optional params are included"""
        # model = kserve.models.v1alpha1_cluster_cached_model.V1alpha1ClusterCachedModel()  # noqa: E501
        if include_optional:
            return V1alpha1ClusterCachedModel(
                api_version="0",
                disabled=True,
                kind="0",
                metadata=None,
                spec=kserve.models.v1alpha1_cluster_cached_model_spec.V1alpha1ClusterCachedModelSpec(
                    cleanup_policy="0",
                    node_group="0",
                    persistent_volume="",
                    persistent_volume_claim="",
                    storage_type="0",
                    storage_uri="0",
                    model_size="1Gi",
                ),
            )
        else:
            return V1alpha1ClusterCachedModel()

    def testV1alpha1ClusterCachedModel(self):
        """Test V1alpha1ClusterCachedModel"""
        inst_req_only = self.make_instance(include_optional=False)
        inst_req_and_optional = self.make_instance(include_optional=True)


if __name__ == "__main__":
    unittest.main()
