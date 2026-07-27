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

import numpy as np
import pandas as pd
import pytest

from kserve.errors import InvalidInput
from kserve.utils.utils import get_predict_input


class TestGetPredictInput:
    def test_mixed_types_in_instances_raises(self):
        payload = {"instances": [[0.0, 141.0, "PPC", "US-MD", 0.14]]}

        with pytest.raises(InvalidInput, match="mix numeric and string values"):
            get_predict_input(payload)

    def test_mixed_types_in_flat_instances_raises(self):
        payload = {"instances": [1.0, "PPC", 2.0]}

        with pytest.raises(InvalidInput, match="mix numeric and string values"):
            get_predict_input(payload)

    def test_numeric_instances_keep_their_dtype(self):
        payload = {"instances": [[0.0, 141.0, 4.0], [1.0, 2.0, 3.0]]}

        result = get_predict_input(payload)

        assert isinstance(result, np.ndarray)
        assert result.dtype == np.float64
        assert result.tolist() == [[0.0, 141.0, 4.0], [1.0, 2.0, 3.0]]

    def test_all_string_instances_are_not_rejected(self):
        payload = {"instances": [["PPC", "OSIOS"], ["SEO", "DESKTOP"]]}

        result = get_predict_input(payload)

        assert isinstance(result, np.ndarray)
        assert result.dtype.kind == "U"
        assert result.tolist() == [["PPC", "OSIOS"], ["SEO", "DESKTOP"]]

    def test_all_bytes_instances_are_not_rejected(self):
        payload = {"instances": [[b"PPC", b"OSIOS"]]}

        result = get_predict_input(payload)

        assert isinstance(result, np.ndarray)
        assert result.dtype.kind == "S"
        assert result.tolist() == [[b"PPC", b"OSIOS"]]

    def test_bytes_mixed_with_numbers_raises(self):
        payload = {"instances": [[b"PPC", 1.0]]}

        with pytest.raises(InvalidInput, match="mix numeric and string values"):
            get_predict_input(payload)

    def test_list_of_strings_is_returned_unchanged(self):
        payload = {"instances": ["first text", "second text"]}

        assert get_predict_input(payload) == ["first text", "second text"]

    def test_dict_instances_preserve_per_field_types(self):
        payload = {"instances": [{"age": [25], "channel": ["PPC"]}]}

        result = get_predict_input(payload)

        assert isinstance(result, pd.DataFrame)
        assert result["age"].dtype == np.int64
        assert result["channel"].dtype == object

    def test_empty_instances(self):
        result = get_predict_input({"instances": []})

        assert isinstance(result, np.ndarray)
        assert result.size == 0
