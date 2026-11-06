"""Lychee installation verifies pinned archives before replacing a tool."""

import hashlib
import io
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest


class InstallerTests(unittest.TestCase):
    def install(self, *, corrupt=False, unsupported=False, through_make=False):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            scripts = root / "hack/setup/cli"
            scripts.mkdir(parents=True)
            shutil.copyfile(
                Path(__file__).parents[1] / "setup/cli/install-lychee.sh",
                scripts / "install-lychee.sh",
            )
            (scripts / "install-lychee.sh").chmod(0o755)
            if through_make:
                shutil.copyfile(
                    Path(__file__).parents[2] / "Makefile.tools.mk",
                    root / "Makefile.tools.mk",
                )
                (root / "Makefile").write_text(
                    "include kserve-deps.env\ninclude Makefile.tools.mk\n"
                )
            (root / "kserve-deps.env").write_text("LYCHEE_VERSION=v0.24.2\n")
            archive_name = "lychee-x86_64-unknown-linux-musl.tar.gz"
            archive = root / archive_name
            binary = b"#!/bin/sh\nprintf 'lychee 0.24.2\\n'\n"
            with tarfile.open(archive, "w:gz") as tar:
                info = tarfile.TarInfo("lychee-x86_64-unknown-linux-musl/lychee")
                info.mode = 0o755
                info.size = len(binary)
                tar.addfile(info, io.BytesIO(binary))
            checksum = hashlib.sha256(archive.read_bytes()).hexdigest()
            (scripts / "lychee-checksums.txt").write_text(
                f"{checksum}  {archive_name}\n"
            )
            if corrupt:
                archive.write_bytes(b"tampered archive")
            tools = root / "fake-tools"
            tools.mkdir()
            (tools / "uname").write_text(
                "#!/bin/sh\nif [ \"$1\" = '-m' ]; then echo x86_64; else echo "
                + ("Plan9" if unsupported else "Linux")
                + "; fi\n"
            )
            (tools / "curl").write_text(
                '#!/bin/sh\nwhile [ $# -gt 0 ]; do if [ "$1" = \'--output\' ]; then cp "$FIXTURE_ARCHIVE" "$2"; exit; fi; shift; done\nexit 1\n'
            )
            for tool in tools.iterdir():
                tool.chmod(0o755)
            output = root / "custom tools"
            if through_make:
                output = root / "custom-tools"
            environment = {
                **os.environ,
                "PATH": str(tools) + os.pathsep + os.environ["PATH"],
                "BIN_DIR": str(output),
                "FIXTURE_ARCHIVE": str(archive),
            }
            result = subprocess.run(
                ["make", "lychee", "LOCALBIN=" + str(output)]
                if through_make
                else ["bash", str(scripts / "install-lychee.sh")],
                cwd=root,
                env=environment,
                capture_output=True,
                text=True,
            )
            if unsupported:
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("Unsupported operating system", result.stderr)
            elif corrupt:
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse((output / "lychee").exists())
                self.assertFalse((output / "lychee").is_symlink())
            else:
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual((output / "lychee").read_bytes(), binary)
                self.assertFalse((root / "bin/lychee").exists())

    def test_custom_destination_is_honored(self):
        self.install()

    def test_tampered_archive_is_rejected(self):
        self.install(corrupt=True)

    def test_unsupported_os_fails_clearly(self):
        self.install(unsupported=True)

    def test_make_rejects_tampered_archive_without_dangling_symlink(self):
        self.install(corrupt=True, through_make=True)

    def test_make_installs_versioned_binary_to_custom_destination(self):
        self.install(through_make=True)
