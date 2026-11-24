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
from unittest.mock import Mock

import pytest

from kserve.protocol.rest import tls_profile
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


def test_apply_tls_13_profile_rejects_cipher_list():
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)

    with pytest.raises(
        ValueError,
        match="TLS 1.3 cipher suites cannot be configured with Python's ssl module",
    ):
        apply_tls_profile(
            context,
            TLSProfile(
                minimum_version=ssl.TLSVersion.TLSv1_3,
                ciphers=("NOT-A-CIPHER",),
            ),
        )


def test_load_tls_profile_provider(monkeypatch):
    ssl_context = Mock()
    provider = Mock()
    provider_factory = Mock(return_value=provider)
    provider_entry_point = Mock()
    provider_entry_point.name = "test"
    provider_entry_point.load.return_value = provider_factory
    entry_point_loader = Mock(return_value=(provider_entry_point,))
    monkeypatch.setattr(tls_profile, "entry_points", entry_point_loader)

    assert tls_profile.load_tls_profile_provider("test", ssl_context) is provider

    entry_point_loader.assert_called_once_with(
        group=tls_profile.TLS_PROFILE_PROVIDER_ENTRY_POINT_GROUP
    )
    provider_factory.assert_called_once_with(ssl_context)


def test_load_tls_profile_provider_rejects_unknown_provider(monkeypatch):
    monkeypatch.setattr(tls_profile, "entry_points", Mock(return_value=()))

    with pytest.raises(ValueError, match="unknown TLS profile provider 'missing'"):
        tls_profile.load_tls_profile_provider("missing", Mock())
