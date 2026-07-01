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

import base64
import json
import os
import unittest.mock as mock

import pytest

from kserve_storage import Storage
from kserve_storage.kserve_storage import (
    _OCI_DOCKER_CONFIG_PATH,
    _OCI_DOCKER_CONFIG_PATH_ENV,
    _detect_goarch,
    _login_from_docker_config,
    _pick_platform,
    _rewrite_with_digest,
    _setup_oci_tls,
)

_IMAGE_MANIFEST = {"mediaType": "application/vnd.oci.image.manifest.v1+json"}
_INDEX = {
    "mediaType": "application/vnd.oci.image.index.v1+json",
    "manifests": [
        {
            "platform": {"architecture": "amd64", "os": "linux"},
            "digest": "sha256:amd64digest",
        },
        {
            "platform": {"architecture": "arm64", "os": "linux"},
            "digest": "sha256:arm64digest",
        },
    ],
}


def _make_client(
    manifest, *, model_files=("model.joblib",), models_dir=True, extra_dirs=("bin",)
):
    """Build a mock OrasClient whose pull() extracts a fake container rootfs."""
    client = mock.MagicMock()
    client.get_manifest.return_value = manifest

    def fake_pull(target=None, outdir=None, config_path=None):
        for d in extra_dirs:
            os.makedirs(os.path.join(outdir, d), exist_ok=True)
        if models_dir:
            md = os.path.join(outdir, "models")
            os.makedirs(md, exist_ok=True)
            for name in model_files:
                with open(os.path.join(md, name), "w") as fh:
                    fh.write("data")
        return []

    client.pull.side_effect = fake_pull
    return client


def test_oci_anonymous_pull_no_config(tmp_path):
    out = str(tmp_path / "out")
    client = _make_client(_IMAGE_MANIFEST)
    with (
        mock.patch("oras.client.OrasClient", return_value=client),
        mock.patch("kserve_storage.kserve_storage.os.path.exists", return_value=False),
    ):
        result = Storage._download_oci("oci://registry.io/mymodel:v1", out)

    assert result == out
    assert os.path.isfile(os.path.join(out, "model.joblib"))
    # No config mounted -> anonymous pull.
    assert client.pull.call_args.kwargs["config_path"] is None
    # get_manifest has no auth param in oras-py; it is called with the target only.
    assert client.get_manifest.call_args.args[0] == "registry.io/mymodel:v1"
    assert "config_path" not in client.get_manifest.call_args.kwargs


def test_oci_with_config(tmp_path):
    out = str(tmp_path / "out")
    client = _make_client(_IMAGE_MANIFEST)

    def exists(path):
        return path == _OCI_DOCKER_CONFIG_PATH

    with (
        mock.patch("oras.client.OrasClient", return_value=client),
        mock.patch("kserve_storage.kserve_storage.os.path.exists", side_effect=exists),
    ):
        Storage._download_oci("oci://registry.io/mymodel:v1", out)

    assert client.pull.call_args.kwargs["config_path"] == _OCI_DOCKER_CONFIG_PATH
    # get_manifest takes no config_path; auth is pre-established via client.login().
    assert "config_path" not in client.get_manifest.call_args.kwargs


def test_oci_multi_arch_index_resolves_to_platform(tmp_path):
    out = str(tmp_path / "out")
    client = _make_client(_INDEX)
    with (
        mock.patch("oras.client.OrasClient", return_value=client),
        mock.patch("kserve_storage.kserve_storage.os.path.exists", return_value=False),
        mock.patch(
            "kserve_storage.kserve_storage.platform.machine", return_value="x86_64"
        ),
    ):
        Storage._download_oci("oci://registry.io/mymodel:v1", out)

    pulled_target = client.pull.call_args.kwargs["target"]
    assert pulled_target == "registry.io/mymodel@sha256:amd64digest"
    assert ":v1" not in pulled_target


def test_oci_multi_arch_index_no_matching_platform_errors(tmp_path):
    out = str(tmp_path / "out")
    index = {
        "mediaType": "application/vnd.oci.image.index.v1+json",
        "manifests": [
            {
                "platform": {"architecture": "arm64", "os": "linux"},
                "digest": "sha256:arm64",
            },
        ],
    }
    client = _make_client(index)
    with (
        mock.patch("oras.client.OrasClient", return_value=client),
        mock.patch("kserve_storage.kserve_storage.os.path.exists", return_value=False),
        mock.patch(
            "kserve_storage.kserve_storage.platform.machine", return_value="x86_64"
        ),
    ):
        with pytest.raises(RuntimeError, match="linux/amd64"):
            Storage._download_oci("oci://registry.io/mymodel:v1", out)
    client.pull.assert_not_called()


def test_oci_no_models_subpath_errors(tmp_path):
    out = str(tmp_path / "out")
    client = _make_client(_IMAGE_MANIFEST, models_dir=False, extra_dirs=("bin", "etc"))
    with (
        mock.patch("oras.client.OrasClient", return_value=client),
        mock.patch("kserve_storage.kserve_storage.os.path.exists", return_value=False),
    ):
        with pytest.raises(RuntimeError, match="no /models/ directory"):
            Storage._download_oci("oci://registry.io/mymodel:v1", out)


def test_oci_rejects_non_oci_uri(tmp_path):
    with pytest.raises(RuntimeError, match="Invalid OCI URI"):
        Storage._download_oci("s3://bucket/model", str(tmp_path))


def test_oci_rewrite_with_digest():
    assert (
        _rewrite_with_digest("reg.io/repo:v1", "sha256:abc") == "reg.io/repo@sha256:abc"
    )
    assert (
        _rewrite_with_digest("reg.io:5000/ns/repo:v1", "sha256:def")
        == "reg.io:5000/ns/repo@sha256:def"
    )
    assert (
        _rewrite_with_digest("reg.io/repo@sha256:old", "sha256:new")
        == "reg.io/repo@sha256:new"
    )
    assert _rewrite_with_digest("repo:v1", "sha256:zzz") == "repo@sha256:zzz"


@pytest.mark.parametrize(
    "machine,expected",
    [
        ("x86_64", "amd64"),
        ("amd64", "amd64"),
        ("aarch64", "arm64"),
        ("arm64", "arm64"),
        ("ppc64le", "ppc64le"),
    ],
)
def test_oci_detect_goarch(machine, expected):
    with mock.patch(
        "kserve_storage.kserve_storage.platform.machine", return_value=machine
    ):
        assert _detect_goarch() == expected


def test_oci_pick_platform_skips_unknown():
    manifests = [
        {
            "platform": {"architecture": "unknown", "os": "unknown"},
            "digest": "sha256:att",
        },
        {"platform": {"architecture": "amd64", "os": "linux"}, "digest": "sha256:img"},
    ]
    assert _pick_platform(manifests, "amd64", "linux")["digest"] == "sha256:img"
    assert _pick_platform(manifests, "ppc64le", "linux") is None


def test_oci_honors_env_var_for_config_path(tmp_path):
    out = str(tmp_path / "out")
    client = _make_client(_IMAGE_MANIFEST)
    with (
        mock.patch("oras.client.OrasClient", return_value=client),
        mock.patch("kserve_storage.kserve_storage.os.path.exists", return_value=True),
        mock.patch.dict(
            os.environ, {_OCI_DOCKER_CONFIG_PATH_ENV: "/custom/path/config.json"}
        ),
    ):
        Storage._download_oci("oci://registry.io/mymodel:v1", out)

    assert client.pull.call_args.kwargs["config_path"] == "/custom/path/config.json"


def test_oci_uses_ca_bundle_from_env(tmp_path):
    ca_cert = tmp_path / "cabundle.crt"
    ca_cert.write_text("---CERT---")
    with mock.patch.dict(
        os.environ, {"CA_BUNDLE_VOLUME_MOUNT_POINT": str(tmp_path)}, clear=False
    ):
        os.environ.pop("REQUESTS_CA_BUNDLE", None)
        _setup_oci_tls()
        assert os.environ["REQUESTS_CA_BUNDLE"] == str(ca_cert)


def test_oci_no_ca_bundle_no_env_change():
    with mock.patch.dict(os.environ, {}, clear=False):
        os.environ.pop("CA_BUNDLE_VOLUME_MOUNT_POINT", None)
        os.environ.pop("REQUESTS_CA_BUNDLE", None)
        _setup_oci_tls()
        assert "REQUESTS_CA_BUNDLE" not in os.environ


def test_oci_login_from_docker_config(tmp_path):
    cfg = tmp_path / "config.json"
    token = base64.b64encode(b"alice:s3cret").decode("utf-8")
    cfg.write_text(json.dumps({"auths": {"registry.io": {"auth": token}}}))

    client = mock.MagicMock()
    _login_from_docker_config(client, "registry.io/ns/model:v1", str(cfg))

    client.login.assert_called_once_with(
        username="alice", password="s3cret", hostname="registry.io"
    )


def test_oci_login_skipped_when_no_matching_registry(tmp_path):
    cfg = tmp_path / "config.json"
    token = base64.b64encode(b"alice:s3cret").decode("utf-8")
    cfg.write_text(json.dumps({"auths": {"other.io": {"auth": token}}}))

    client = mock.MagicMock()
    _login_from_docker_config(client, "registry.io/ns/model:v1", str(cfg))

    client.login.assert_not_called()


def test_oci_login_skipped_for_credential_helper(tmp_path):
    # An entry without "auth" (credentials served by a helper) must not crash and
    # must not attempt a username/password login.
    cfg = tmp_path / "config.json"
    cfg.write_text(json.dumps({"auths": {"registry.io": {}}}))

    client = mock.MagicMock()
    _login_from_docker_config(client, "registry.io/ns/model:v1", str(cfg))

    client.login.assert_not_called()


def test_oci_login_silent_on_malformed_config(tmp_path):
    client = mock.MagicMock()

    # Missing file -> OSError swallowed, no login.
    missing = str(tmp_path / "does-not-exist.json")
    _login_from_docker_config(client, "registry.io/ns/model:v1", missing)
    client.login.assert_not_called()

    # Malformed JSON -> ValueError swallowed, no login.
    bad = tmp_path / "bad.json"
    bad.write_text("{not json")
    _login_from_docker_config(client, "registry.io/ns/model:v1", str(bad))
    client.login.assert_not_called()


def test_oras_get_manifest_signature_drift():
    # If oras-py ever adds a config_path/auth param to get_manifest, revisit
    # _download_oci: we deliberately pre-establish auth via client.login()
    # because the current API has no auth param on get_manifest.
    import inspect

    import oras.client

    sig = inspect.signature(oras.client.OrasClient.get_manifest)
    assert "config_path" not in sig.parameters
