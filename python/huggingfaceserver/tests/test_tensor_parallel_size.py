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

import pytest

from huggingfaceserver.vllm.utils import cli_flag_given, resolve_tensor_parallel_size


@pytest.mark.parametrize(
    "argv,tp,pp,gpus,expected",
    [
        ([], 1, 1, 4, 4),  # no flag: use every visible GPU
        ([], 1, 2, 4, 2),  # no flag: GPUs left per pipeline stage
        (["--tensor-parallel-size", "1"], 1, 1, 3, 1),
        (["--tensor_parallel_size=2"], 2, 2, 4, 2),
        (["-tp", "2"], 2, 1, 8, 2),
        (["--pipeline-parallel-size", "3"], 1, 3, 2, 1),
    ],
)
def test_resolve_tensor_parallel_size(argv, tp, pp, gpus, expected):
    assert resolve_tensor_parallel_size(tp, pp, gpus, argv) == expected


def test_cli_flag_given_ignores_values():
    assert not cli_flag_given(("--tensor-parallel-size",), ["--model", "--tensor-parallel-size-x"])
