import hashlib
from pathlib import Path

from kserve_storage.logging import logger


def get_single_artifact(download_dir: str) -> str:
    """
    Returns the path to the single downloaded artifact.

    Raises:
        RuntimeError:
            - if no files are found
            - if multiple files/directories are found
    """
    path = Path(download_dir)

    if not path.exists():
        raise RuntimeError(f"Download path does not exist: {download_dir}")

    entries = list(path.iterdir())

    if len(entries) != 1:
        raise RuntimeError(
            f"Expected one downloaded artifact, found {len(entries)} in '{download_dir}'. "
            "Digest verification currently supports only single-file artifacts."
        )

    artifact = entries[0]

    if artifact.is_dir():
        raise RuntimeError(
            f"Expected a single downloaded file, but found directory '{artifact.name}'. "
            "Directory artifacts are not supported."
        )

    return str(artifact)


def sha256_file(path: Path) -> str:
    """Compute SHA-256 of a single file, returning a 'sha256:<hex>' string."""
    h = hashlib.sha256()

    with path.open("rb") as f:
        while True:
            chunk = f.read(1024 * 1024)
            if not chunk:
                break
            h.update(chunk)

    return f"sha256:{h.hexdigest()}"


def verify_digest(path: str, expected_digest: str) -> None:
    """Verify the SHA-256 digest of the artifact at *path* against *expected_digest*.

    *expected_digest* may be supplied with or without the ``sha256:`` prefix and
    is compared case-insensitively.

    Raises:
        RuntimeError: if the computed digest does not match the expected digest.
    """
    actual = sha256_file(Path(path))

    expected_normalized = expected_digest.lower()
    actual_normalized = actual.lower()

    if expected_normalized.startswith("sha256:"):
        expected_normalized = expected_normalized[len("sha256:"):]

    if actual_normalized.startswith("sha256:"):
        actual_normalized = actual_normalized[len("sha256:"):]

    if actual_normalized != expected_normalized:
        raise RuntimeError(
            f"Digest verification failed. Expected {expected_digest}, got {actual_normalized}"
        )

    logger.info("Digest verification succeeded.")