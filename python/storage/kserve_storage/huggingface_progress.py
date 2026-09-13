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

import os
import threading
import time

from huggingface_hub.utils import tqdm as huggingface_tqdm

from kserve_storage.logging import logger

_HEARTBEAT_SECONDS = 60.0


def get_huggingface_repo_size_bytes(
    repo_id: str,
    revision: str | None = None,
    allow_patterns: list[str] | None = None,
    ignore_patterns: list[str] | None = None,
) -> int | None:
    from huggingface_hub import HfApi
    from huggingface_hub.utils import filter_repo_objects

    try:
        model_info = HfApi().model_info(
            repo_id=repo_id,
            revision=revision,
            files_metadata=True,
        )
    except Exception as error:
        logger.warning(
            "Unable to determine Hugging Face download size; "
            "continuing without percentage: %s",
            error,
        )
        return None

    siblings = model_info.siblings or []
    selected_files = filter_repo_objects(
        siblings,
        allow_patterns=allow_patterns,
        ignore_patterns=ignore_patterns,
        key=lambda sibling: sibling.rfilename,
    )
    sizes = [sibling.size for sibling in selected_files]
    if not sizes or any(size is None for size in sizes):
        return None

    return sum(sizes)


def _get_local_size_bytes(local_dir: str) -> int:
    total_bytes = 0
    directories = [local_dir]

    while directories:
        directory = directories.pop()
        try:
            with os.scandir(directory) as entries:
                for entry in entries:
                    try:
                        if entry.is_dir(follow_symlinks=False):
                            directories.append(entry.path)
                        elif entry.is_file(follow_symlinks=False):
                            stat = entry.stat(follow_symlinks=False)
                            allocated_blocks = getattr(stat, "st_blocks", None)
                            total_bytes += (
                                allocated_blocks * 512
                                if allocated_blocks is not None
                                else stat.st_size
                            )
                    except OSError:
                        continue
        except OSError:
            continue

    return total_bytes


class HuggingFaceLogProgress(huggingface_tqdm):
    """Render Hugging Face snapshot progress as line-oriented logs."""

    def __init__(
        self,
        *args,
        local_dir: str | None = None,
        total_size_bytes: int | None = None,
        **kwargs,
    ):
        self.local_dir = local_dir
        self.total_size_bytes = total_size_bytes
        self._heartbeat_stop = threading.Event()
        self._heartbeat_thread = None
        self._progress_lock = threading.Lock()
        self._last_log_at = time.monotonic()
        self._kserve_initialized = False
        self._kserve_closed = False
        kwargs.setdefault("mininterval", _HEARTBEAT_SECONDS)
        kwargs.setdefault("maxinterval", _HEARTBEAT_SECONDS)
        kwargs.setdefault("leave", False)
        super().__init__(*args, **kwargs)
        self._kserve_initialized = True

        if not self.disable:
            self._emit_progress("started")
            self._start_heartbeat()

    def display(self, msg=None, pos=None):
        if self._kserve_initialized and not self._kserve_closed:
            self._log_progress_if_due("running")

    def _start_heartbeat(self):
        self._heartbeat_thread = threading.Thread(
            target=self._heartbeat,
            name="huggingface-download-progress",
            daemon=True,
        )
        self._heartbeat_thread.start()

    def _heartbeat(self):
        while True:
            with self._progress_lock:
                next_log_in = max(
                    0.0,
                    _HEARTBEAT_SECONDS - (time.monotonic() - self._last_log_at),
                )
            if self._heartbeat_stop.wait(next_log_in):
                return
            self._emit_progress("running")

    def _log_progress_if_due(self, state: str):
        with self._progress_lock:
            if time.monotonic() - self._last_log_at < _HEARTBEAT_SECONDS:
                return
        self._emit_progress(state)

    def _emit_progress(self, state: str):
        now = time.monotonic()
        with self._progress_lock:
            self._last_log_at = now
        total = self.total if self.total is not None else "unknown"
        local_size_bytes = (
            _get_local_size_bytes(self.local_dir) if self.local_dir is not None else 0
        )
        local_size_gb = local_size_bytes / 1_000_000_000

        if self.total_size_bytes:
            total_size_gb = self.total_size_bytes / 1_000_000_000
            progress_percent = min(
                local_size_bytes / self.total_size_bytes * 100,
                100.0,
            )
            if state == "completed":
                progress_percent = 100.0
            message = (
                f"HF progress: {local_size_gb:.2f}/{total_size_gb:.2f}GB "
                f"({progress_percent:.1f}%), {self.n}/{total} files"
            )
        else:
            message = f"HF progress: {local_size_gb:.2f}GB, {self.n}/{total} files"

        if state == "completed":
            message += ", complete"
        elif state == "stopped":
            message += ", stopped"

        logger.info(message)

    def close(self):
        if not getattr(self, "_kserve_initialized", False):
            return super().close()
        if self._kserve_closed:
            return

        self._kserve_closed = True
        self._heartbeat_stop.set()
        if self._heartbeat_thread is not None:
            self._heartbeat_thread.join(timeout=1)

        progress_was_disabled = self.disable
        super().close()
        if not progress_was_disabled:
            state = (
                "completed"
                if self.total is not None and self.n >= self.total
                else "stopped"
            )
            self._emit_progress(state)


def create_huggingface_log_progress(
    local_dir: str,
    total_size_bytes: int | None = None,
) -> type[HuggingFaceLogProgress]:
    class BoundHuggingFaceLogProgress(HuggingFaceLogProgress):
        def __init__(self, *args, **kwargs):
            super().__init__(
                *args,
                local_dir=local_dir,
                total_size_bytes=total_size_bytes,
                **kwargs,
            )

    return BoundHuggingFaceLogProgress
