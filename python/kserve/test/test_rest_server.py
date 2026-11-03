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

import ssl
from datetime import datetime, timedelta, timezone
from unittest.mock import AsyncMock, Mock

import pytest
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import NameOID

from kserve.protocol.rest import server as rest_mod
from kserve.protocol.rest.tls_profile import TLSProfile, apply_tls_profile


def _write_tls_certificate(tmp_path):
    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    subject = issuer = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "localhost")])
    certificate = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(issuer)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(datetime.now(timezone.utc) - timedelta(minutes=1))
        .not_valid_after(datetime.now(timezone.utc) + timedelta(days=1))
        .sign(key, hashes.SHA256())
    )
    cert_path = tmp_path / "tls.crt"
    key_path = tmp_path / "tls.key"
    cert_path.write_bytes(certificate.public_bytes(serialization.Encoding.PEM))
    key_path.write_bytes(
        key.private_bytes(
            serialization.Encoding.PEM,
            serialization.PrivateFormat.PKCS8,
            serialization.NoEncryption(),
        )
    )
    return cert_path, key_path


@pytest.mark.parametrize(
    "loop_value,expected",
    [
        ("auto", "auto"),
        ("asyncio", "asyncio"),
        ("uvloop", "uvloop"),
        ("invalid-value", "auto"),  # invalid falls back to 'auto'
    ],
)
def test_config_loop_value(loop_value, expected, monkeypatch):
    monkeypatch.setattr(rest_mod.RESTServer, "create_application", lambda self: None)
    data_plane = Mock()
    model_repo_ext = Mock()

    rs = rest_mod.RESTServer(
        app="dummy:app",
        data_plane=data_plane,
        model_repository_extension=model_repo_ext,
        http_port=8080,
        event_loop=loop_value,
    )

    assert rs.config.loop == expected


@pytest.mark.parametrize(
    "timeout_keep_alive,expected",
    [
        (65, 65),  # default value
        (120, 120),  # custom value
        (5, 5),  # uvicorn default
    ],
)
def test_config_timeout_keep_alive(timeout_keep_alive, expected, monkeypatch):
    monkeypatch.setattr(rest_mod.RESTServer, "create_application", lambda self: None)
    data_plane = Mock()
    model_repo_ext = Mock()

    rs = rest_mod.RESTServer(
        app="dummy:app",
        data_plane=data_plane,
        model_repository_extension=model_repo_ext,
        http_port=8080,
        timeout_keep_alive=timeout_keep_alive,
    )

    assert rs.config.timeout_keep_alive == expected


def test_config_timeout_keep_alive_default(monkeypatch):
    monkeypatch.setattr(rest_mod.RESTServer, "create_application", lambda self: None)
    data_plane = Mock()
    model_repo_ext = Mock()

    rs = rest_mod.RESTServer(
        app="dummy:app",
        data_plane=data_plane,
        model_repository_extension=model_repo_ext,
        http_port=8080,
    )

    assert rs.config.timeout_keep_alive == 65


@pytest.mark.parametrize(
    "ssl_certfile,ssl_keyfile",
    [("/etc/tls/tls.crt", None), (None, "/etc/tls/tls.key")],
)
def test_rejects_partial_ssl_configuration(ssl_certfile, ssl_keyfile):
    with pytest.raises(
        ValueError, match="ssl_certfile and ssl_keyfile must be configured together"
    ):
        rest_mod.RESTServer(
            app="dummy:app",
            data_plane=Mock(),
            model_repository_extension=Mock(),
            http_port=8080,
            ssl_certfile=ssl_certfile,
            ssl_keyfile=ssl_keyfile,
        )


@pytest.mark.asyncio
async def test_tls_profile_provider_requires_ssl(monkeypatch):
    monkeypatch.setattr(rest_mod.RESTServer, "create_application", lambda self: None)
    server_serve = AsyncMock()
    monkeypatch.setattr(rest_mod.uvicorn.Server, "serve", server_serve)
    provider_loader = Mock()
    monkeypatch.setattr(rest_mod, "load_tls_profile_provider", provider_loader)
    monkeypatch.setenv(rest_mod.TLS_PROFILE_PROVIDER_ENV, "test")

    rs = rest_mod.RESTServer(
        app="dummy:app",
        data_plane=Mock(),
        model_repository_extension=Mock(),
        http_port=8080,
    )

    def load_config():
        rs.config.loaded = True
        rs.config.ssl = None

    monkeypatch.setattr(rs.config, "load", load_config)

    with pytest.raises(
        ValueError,
        match=(
            "KSERVE_TLS_PROFILE_PROVIDER requires TLS to be configured "
            "with ssl_certfile and ssl_keyfile"
        ),
    ):
        await rs.start()

    provider_loader.assert_not_called()
    server_serve.assert_not_called()


@pytest.mark.asyncio
async def test_ssl_cert_refresher_lifecycle(monkeypatch):
    monkeypatch.setattr(rest_mod.RESTServer, "create_application", lambda self: None)
    monkeypatch.setattr(rest_mod.uvicorn.Server, "serve", AsyncMock())
    refresher = Mock()
    refresher_class = Mock(return_value=refresher)
    monkeypatch.setattr(rest_mod, "SSLCertRefresher", refresher_class)
    ssl_context = Mock()

    rs = rest_mod.RESTServer(
        app="dummy:app",
        data_plane=Mock(),
        model_repository_extension=Mock(),
        http_port=8443,
        ssl_certfile="/etc/tls/tls.crt",
        ssl_keyfile="/etc/tls/tls.key",
    )

    def load_config():
        rs.config.loaded = True
        rs.config.ssl = ssl_context

    monkeypatch.setattr(rs.config, "load", load_config)

    await rs.start()

    refresher_class.assert_called_once_with(
        ssl_context=ssl_context,
        key_path="/etc/tls/tls.key",
        cert_path="/etc/tls/tls.crt",
    )
    refresher.stop.assert_called_once_with()


@pytest.mark.asyncio
async def test_tls_profile_provider_lifecycle(monkeypatch):
    monkeypatch.setattr(rest_mod.RESTServer, "create_application", lambda self: None)
    monkeypatch.setattr(rest_mod.uvicorn.Server, "serve", AsyncMock())
    ssl_context = Mock()
    provider = Mock()
    monkeypatch.setenv(rest_mod.TLS_PROFILE_PROVIDER_ENV, "test")
    provider_loader = Mock(return_value=provider)
    monkeypatch.setattr(rest_mod, "load_tls_profile_provider", provider_loader)

    rs = rest_mod.RESTServer(
        app="dummy:app",
        data_plane=Mock(),
        model_repository_extension=Mock(),
        http_port=8443,
        ssl_certfile="/etc/tls/tls.crt",
        ssl_keyfile="/etc/tls/tls.key",
    )

    def load_config():
        rs.config.loaded = True
        rs.config.ssl = ssl_context

    monkeypatch.setattr(rs.config, "load", load_config)

    await rs.start()

    provider_loader.assert_called_once_with("test", ssl_context)
    provider.start.assert_called_once_with()
    provider.stop.assert_called_once_with()


@pytest.mark.asyncio
async def test_tls_profile_provider_receives_loaded_uvicorn_ssl_context(
    monkeypatch, tmp_path
):
    monkeypatch.setattr(rest_mod.RESTServer, "create_application", lambda self: None)
    monkeypatch.setattr(rest_mod.uvicorn.Server, "serve", AsyncMock())
    monkeypatch.setattr(rest_mod, "SSLCertRefresher", Mock())
    monkeypatch.setenv(rest_mod.TLS_PROFILE_PROVIDER_ENV, "test")
    provider = Mock()

    def load_provider(name, ssl_context):
        apply_tls_profile(
            ssl_context,
            TLSProfile(
                minimum_version=ssl.TLSVersion.TLSv1_2,
                ciphers=("ECDHE-RSA-AES128-GCM-SHA256",),
            ),
        )
        return provider

    provider_loader = Mock(side_effect=load_provider)
    monkeypatch.setattr(rest_mod, "load_tls_profile_provider", provider_loader)
    cert_path, key_path = _write_tls_certificate(tmp_path)
    rs = rest_mod.RESTServer(
        app=rest_mod.fastapi.FastAPI(),
        data_plane=Mock(),
        model_repository_extension=Mock(),
        http_port=8443,
        ssl_certfile=str(cert_path),
        ssl_keyfile=str(key_path),
    )

    await rs.start()

    provider_loader.assert_called_once_with("test", rs.config.ssl)
    assert isinstance(rs.config.ssl, ssl.SSLContext)
    assert rs.config.ssl.minimum_version == ssl.TLSVersion.TLSv1_2
    assert "ECDHE-RSA-AES128-GCM-SHA256" in {
        cipher["name"] for cipher in rs.config.ssl.get_ciphers()
    }
    provider.start.assert_called_once_with()
    provider.stop.assert_called_once_with()


@pytest.mark.asyncio
async def test_tls_profile_provider_start_failure_stops_refreshers(monkeypatch):
    monkeypatch.setattr(rest_mod.RESTServer, "create_application", lambda self: None)
    cert_refresher = Mock()
    monkeypatch.setattr(rest_mod, "SSLCertRefresher", Mock(return_value=cert_refresher))
    provider = Mock()
    provider.start.side_effect = RuntimeError("provider failed")
    ssl_context = Mock()
    monkeypatch.setenv(rest_mod.TLS_PROFILE_PROVIDER_ENV, "test")
    monkeypatch.setattr(
        rest_mod, "load_tls_profile_provider", Mock(return_value=provider)
    )

    rs = rest_mod.RESTServer(
        app="dummy:app",
        data_plane=Mock(),
        model_repository_extension=Mock(),
        http_port=8443,
        ssl_certfile="/etc/tls/tls.crt",
        ssl_keyfile="/etc/tls/tls.key",
    )

    def load_config():
        rs.config.loaded = True
        rs.config.ssl = ssl_context

    monkeypatch.setattr(rs.config, "load", load_config)

    with pytest.raises(RuntimeError, match="provider failed"):
        await rs.start()

    cert_refresher.stop.assert_called_once_with()
    provider.stop.assert_called_once_with()
