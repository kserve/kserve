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
from collections.abc import Sequence
from dataclasses import dataclass
from importlib.metadata import entry_points
from typing import Protocol


TLS_PROFILE_PROVIDER_ENTRY_POINT_GROUP = "kserve.tls_profile_providers"
TLS_PROFILE_PROVIDER_ENV = "KSERVE_TLS_PROFILE_PROVIDER"


@dataclass(frozen=True)
class TLSProfile:
    """TLS settings to apply to a server SSL context."""

    minimum_version: ssl.TLSVersion
    ciphers: Sequence[str] = ()


def apply_tls_profile(ssl_context: ssl.SSLContext, profile: TLSProfile) -> None:
    """Apply a TLS profile to an active SSL context."""
    if profile.ciphers and profile.minimum_version >= ssl.TLSVersion.TLSv1_3:
        raise ValueError(
            "TLS 1.3 cipher suites cannot be configured with Python's ssl module"
        )

    ssl_context.minimum_version = profile.minimum_version
    if profile.ciphers:
        ssl_context.set_ciphers(":".join(profile.ciphers))


class TLSProfileProvider(Protocol):
    """Lifecycle for a source that configures and refreshes an SSL context."""

    def start(self) -> None: ...

    def stop(self) -> None: ...


def load_tls_profile_provider(
    name: str, ssl_context: ssl.SSLContext
) -> TLSProfileProvider:
    """Load a TLS profile provider registered under the KServe entry-point group."""
    for provider_entry_point in entry_points(
        group=TLS_PROFILE_PROVIDER_ENTRY_POINT_GROUP
    ):
        if provider_entry_point.name == name:
            return provider_entry_point.load()(ssl_context)

    raise ValueError(f"unknown TLS profile provider {name!r}")
