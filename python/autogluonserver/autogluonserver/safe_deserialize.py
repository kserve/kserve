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

from __future__ import annotations

import io
import json
import os
import pickletools
import zipfile
from dataclasses import dataclass
from enum import Enum
from pathlib import Path
from typing import Iterable, List, Sequence

from kserve.errors import InferenceError
from kserve.logging import logger

ENV_SAFE_LOAD_MODE = "AUTOGLUON_SAFE_LOAD_MODE"
ENV_ALLOWED_MODULES = "AUTOGLUON_SAFE_LOAD_ALLOWED_MODULES"
ENV_SCAN_PATTERNS = "AUTOGLUON_SAFE_LOAD_SCAN_PATTERNS"
ENV_LOG_DENIED_MAX = "AUTOGLUON_SAFE_LOAD_LOG_DENIED_MAX"

DEFAULT_SCAN_PATTERNS: tuple[str, ...] = ("*.pkl", "*.pickle", "*.joblib")
# Default module-prefix allowlist for legitimate AutoGluon artifacts.
# These prefixes are intentionally broad enough to cover common tabular/time-series
# stacks shipped in the runtime image.
#
# Notes on sensitive-looking entries:
# - "cloudpickle": required because AutoGluon may persist objects serialized
#   through cloudpickle in some training flows.
# - "inspect": used by parts of AutoGluon/fastai internals during object restore.
# - "pathlib": appears in persisted path objects.
# - "random": used by model components/state in some pipelines.
# They remain allowlisted for compatibility; operators can still tighten policy by
# combining restrictive model sources with AUTOGLUON_SAFE_LOAD_MODE=enforce.
DEFAULT_ALLOWED_MODULE_PREFIXES: tuple[str, ...] = (
    "autogluon",
    "numpy",
    "pandas",
    "scipy",
    "sklearn",
    "xgboost",
    "lightgbm",
    "catboost",
    "torch",
    "collections",
    "builtins",
    "__builtin__",
    "_codecs",
    "copyreg",
    "datetime",
    "inspect",
    "joblib",
    "cloudpickle",
    "pathlib",
    "random",
    "typing",
    "fastai",
    "fastcore",
    "fasttransform",
)

_STACK_MARKER = object()


class SafeLoadMode(str, Enum):
    OFF = "off"
    PERMISSIVE = "permissive"
    ENFORCE = "enforce"


@dataclass(frozen=True)
class PickleGlobalRef:
    file_path: str
    module: str
    name: str


@dataclass(frozen=True)
class SafeLoadScanResult:
    status: str
    scanned_files: List[str]
    forbidden_refs: List[PickleGlobalRef]
    errors: List[str]


def get_safe_load_mode() -> SafeLoadMode:
    raw = os.environ.get(ENV_SAFE_LOAD_MODE, SafeLoadMode.OFF.value).strip().lower()
    if raw in {m.value for m in SafeLoadMode}:
        return SafeLoadMode(raw)
    logger.warning(
        "Unknown %s value %r. Falling back to %s.",
        ENV_SAFE_LOAD_MODE,
        raw,
        SafeLoadMode.OFF.value,
    )
    return SafeLoadMode.OFF


def get_scan_patterns() -> tuple[str, ...]:
    raw = os.environ.get(ENV_SCAN_PATTERNS, "").strip()
    if not raw:
        return DEFAULT_SCAN_PATTERNS
    return tuple(_parse_list_env(raw, fallback=DEFAULT_SCAN_PATTERNS))


def get_allowed_module_prefixes() -> tuple[str, ...]:
    base = list(DEFAULT_ALLOWED_MODULE_PREFIXES)
    raw = os.environ.get(ENV_ALLOWED_MODULES, "").strip()
    if raw:
        base.extend(_parse_list_env(raw, fallback=()))
    deduped: List[str] = []
    seen: set[str] = set()
    for entry in base:
        val = entry.strip()
        if val and val not in seen:
            seen.add(val)
            deduped.append(val)
    return tuple(deduped)


def validate_model_artifacts_for_safe_load(model_dir: str, *, context: str = "") -> None:
    mode = get_safe_load_mode()
    if mode is SafeLoadMode.OFF:
        return

    result = scan_model_artifacts(
        model_dir,
        allow_module_prefixes=get_allowed_module_prefixes(),
        scan_patterns=get_scan_patterns(),
    )
    prefix = f"[{context}] " if context else ""

    if result.status in {"ok", "missing_artifacts"}:
        if result.status == "missing_artifacts":
            logger.warning(
                "%sSafe-load scan found no files matching patterns %s under %s.",
                prefix,
                list(get_scan_patterns()),
                model_dir,
            )
        return

    denied_limit = _get_denied_log_limit()
    denied = result.forbidden_refs[:denied_limit]
    denied_fmt = ", ".join(f"{r.module}.{r.name} ({r.file_path})" for r in denied)
    if len(result.forbidden_refs) > denied_limit:
        denied_fmt += f", ... ({len(result.forbidden_refs) - denied_limit} more)"
    error_fmt = "; ".join(result.errors[:denied_limit])
    msg = (
        f"{prefix}Safe-load validation status={result.status} for model directory {model_dir}. "
        f"Forbidden references: [{denied_fmt}]."
    )
    if error_fmt:
        msg += f" Scan errors: {error_fmt}."

    if mode is SafeLoadMode.PERMISSIVE:
        logger.warning(msg)
        return
    raise InferenceError(msg)


def assert_safe_autogluon_artifact(model_dir: str) -> None:
    """
    Backward-compatible wrapper.
    """
    validate_model_artifacts_for_safe_load(model_dir, context="autogluon_artifact")


def scan_model_artifacts(
    model_dir: str,
    *,
    allow_module_prefixes: Sequence[str],
    scan_patterns: Sequence[str],
) -> SafeLoadScanResult:
    model_path = Path(model_dir)
    if not model_path.exists() or not model_path.is_dir():
        return SafeLoadScanResult(
            status="missing_artifacts",
            scanned_files=[],
            forbidden_refs=[],
            errors=[f"{model_dir} is not a directory"],
        )

    files = _resolve_scan_files(model_path, scan_patterns)
    if not files:
        return SafeLoadScanResult(
            status="missing_artifacts", scanned_files=[], forbidden_refs=[], errors=[]
        )

    forbidden_refs: List[PickleGlobalRef] = []
    errors: List[str] = []
    scanned_files: List[str] = []
    for file_path in files:
        scanned_files.append(str(file_path))
        refs, scan_errors = _scan_pickle_file(file_path)
        errors.extend(scan_errors)
        for module, name in refs:
            if not _is_allowed_module(module, allow_module_prefixes):
                forbidden_refs.append(
                    PickleGlobalRef(file_path=str(file_path), module=module, name=name)
                )

    if forbidden_refs:
        return SafeLoadScanResult(
            status="forbidden_refs",
            scanned_files=scanned_files,
            forbidden_refs=forbidden_refs,
            errors=errors,
        )
    if errors:
        return SafeLoadScanResult(
            status="scan_error",
            scanned_files=scanned_files,
            forbidden_refs=[],
            errors=errors,
        )
    return SafeLoadScanResult(
        status="ok", scanned_files=scanned_files, forbidden_refs=[], errors=[]
    )


def _resolve_scan_files(model_path: Path, scan_patterns: Sequence[str]) -> list[Path]:
    matches: List[Path] = []
    seen: set[Path] = set()
    for pattern in scan_patterns:
        pat = pattern.strip()
        if not pat:
            continue
        for match in model_path.rglob(pat):
            if match.is_file() and match not in seen:
                seen.add(match)
                matches.append(match)
    return sorted(matches)


def _scan_pickle_file(file_path: Path) -> tuple[list[tuple[str, str]], list[str]]:
    try:
        payload = file_path.read_bytes()
    except OSError as e:
        return [], [f"{file_path}: {e}"]
    if not payload:
        return [], []
    if _looks_like_zip(payload):
        return _scan_zip_container(payload, file_path)
    return _scan_pickle_bytes(payload, file_path)


def _scan_zip_container(
    payload: bytes, file_path: Path
) -> tuple[list[tuple[str, str]], list[str]]:
    refs: list[tuple[str, str]] = []
    errors: list[str] = []
    try:
        with zipfile.ZipFile(io.BytesIO(payload)) as zf:
            member_names = [
                name
                for name in zf.namelist()
                if name.endswith(".pkl") or name.endswith(".pickle")
            ]
            for member_name in member_names:
                member_payload = zf.read(member_name)
                mrefs, merrs = _scan_pickle_bytes(
                    member_payload, Path(f"{file_path}:{member_name}")
                )
                refs.extend(mrefs)
                errors.extend(merrs)
    except zipfile.BadZipFile as e:
        errors.append(f"{file_path}: cannot parse zip container ({e})")
    except OSError as e:
        errors.append(f"{file_path}: cannot read zip container ({e})")
    return refs, errors


def _scan_pickle_bytes(
    payload: bytes, source: Path
) -> tuple[list[tuple[str, str]], list[str]]:
    """
    Scan pickle bytecode for explicit GLOBAL/STACK_GLOBAL module references.

    This is static analysis over pickle opcodes. It can detect direct module/name
    references embedded in the stream, but it is not a full sandbox and cannot
    prove deserialization safety. In particular, it does not detect malicious
    behavior hidden in legitimate objects (e.g. custom __reduce__/__setstate__
    logic) or exploits inside allowed third-party modules.
    """
    refs: list[tuple[str, str]] = []
    errors: list[str] = []
    stack: list[object] = []
    try:
        for op, arg, _ in pickletools.genops(payload):
            name = op.name
            if name == "GLOBAL":
                mod, cls = _split_global_arg(str(arg))
                refs.append((mod, cls))
                continue
            if name in {"SHORT_BINUNICODE", "BINUNICODE", "UNICODE"}:
                stack.append(str(arg))
                continue
            if name == "STRING":
                stack.append(_strip_py2_string_quotes(str(arg)))
                continue
            if name == "STACK_GLOBAL":
                if len(stack) >= 2 and isinstance(stack[-2], str) and isinstance(
                    stack[-1], str
                ):
                    refs.append((stack[-2], stack[-1]))
                continue
            if name == "MARK":
                stack.append(_STACK_MARKER)
                continue
            if name == "POP":
                if stack:
                    stack.pop()
                continue
            if name == "POP_MARK":
                while stack:
                    item = stack.pop()
                    if item is _STACK_MARKER:
                        break
                continue
            if name == "DUP":
                if stack:
                    stack.append(stack[-1])
                continue
            if name in {
                "EMPTY_TUPLE",
                "EMPTY_LIST",
                "EMPTY_DICT",
                "EMPTY_SET",
                "NEWTRUE",
                "NEWFALSE",
                "NONE",
                "BININT",
                "BININT1",
                "BININT2",
                "LONG",
                "LONG1",
                "LONG4",
                "BINFLOAT",
                "FLOAT",
            }:
                stack.append(object())
                continue
            stack.clear()
    except Exception as e:  # pragma: no cover
        errors.append(f"{source}: cannot parse pickle stream ({e})")
    return refs, errors


def _split_global_arg(arg: str) -> tuple[str, str]:
    parts = arg.split(" ", 1)
    if len(parts) != 2:
        return arg.strip(), "<unknown>"
    return parts[0].strip(), parts[1].strip()


def _strip_py2_string_quotes(arg: str) -> str:
    s = arg.strip()
    if len(s) >= 2 and s[0] == s[-1] and s[0] in {"'", '"'}:
        return s[1:-1]
    return s


def _is_allowed_module(module: str, allowed_prefixes: Iterable[str]) -> bool:
    for prefix in allowed_prefixes:
        normalized = prefix.strip()
        if not normalized:
            continue
        if module == normalized or module.startswith(normalized + "."):
            return True
    return False


def _parse_list_env(raw: str, *, fallback: Sequence[str]) -> list[str]:
    try:
        parsed = json.loads(raw)
        if isinstance(parsed, list):
            return [str(item).strip() for item in parsed if str(item).strip()]
        if isinstance(parsed, str) and parsed.strip():
            return [parsed.strip()]
    except json.JSONDecodeError:
        pass
    values = [item.strip() for item in raw.split(",") if item.strip()]
    if values:
        return values
    return [item for item in fallback]


def _get_denied_log_limit() -> int:
    raw = os.environ.get(ENV_LOG_DENIED_MAX, "10").strip()
    try:
        value = int(raw)
    except ValueError:
        return 10
    return max(1, value)


def _looks_like_zip(payload: bytes) -> bool:
    return payload.startswith(b"PK\x03\x04")
