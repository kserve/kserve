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

import ssl

from kserve.protocol.rest.tls_profile import TLSProfile, apply_tls_profile


def test_apply_tls_profile_sets_minimum_version_and_ciphers():
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    profile = TLSProfile(
        minimum_version=ssl.TLSVersion.TLSv1_2,
        ciphers=("ECDHE-RSA-AES128-GCM-SHA256",),
    )

    apply_tls_profile(context, profile)

    assert context.minimum_version == ssl.TLSVersion.TLSv1_2
    assert "ECDHE-RSA-AES128-GCM-SHA256" in {
        cipher["name"] for cipher in context.get_ciphers()
    }


def test_apply_tls_13_profile_ignores_cipher_list():
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)

    apply_tls_profile(
        context,
        TLSProfile(
            minimum_version=ssl.TLSVersion.TLSv1_3,
            ciphers=("NOT-A-CIPHER",),
        ),
    )

    assert context.minimum_version == ssl.TLSVersion.TLSv1_3
