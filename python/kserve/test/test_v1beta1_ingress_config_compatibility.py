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

from kserve import V1beta1IngressConfig
from kserve.configuration import Configuration


def test_previous_positional_constructor_signature_is_preserved():
    values = [f"value-{index}" for index in range(18)]
    configuration = Configuration()

    config = V1beta1IngressConfig(*values, configuration)

    assert config.local_gateway == "value-12"
    assert config.url_scheme == "value-17"
    assert config.local_vars_configuration is configuration


def test_tls_profile_fields_are_keyword_only():
    config = V1beta1IngressConfig(
        llm_inference_service_tls_min_version="VersionTLS12",
        llm_inference_service_tls_cipher_suites="TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
    )

    assert config.llm_inference_service_tls_min_version == "VersionTLS12"
    assert config.llm_inference_service_tls_cipher_suites.startswith("TLS_ECDHE_RSA")
