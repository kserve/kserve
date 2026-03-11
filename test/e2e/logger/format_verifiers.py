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
import csv
import io
import json

import pyarrow.parquet as pq

LOG_RECORD_COLUMNS = [
    "url",
    "bytes",
    "contentType",
    "reqType",
    "id",
    "sourceUri",
    "inferenceService",
    "namespace",
    "component",
    "endpoint",
    "metadata",
    "annotations",
    "certName",
    "tlsSkipVerify",
]


def verify_json_object(data: bytes, expected_records: int):
    """Parse JSON marshaller output and validate structure.

    For batch size 1 (single record), the JSON marshaller outputs a single
    LogRequest object. For batch size > 1, it outputs a JSON array.
    """
    parsed = json.loads(data)

    if expected_records == 1:
        assert isinstance(parsed, dict), (
            f"Single-record JSON should be an object, got {type(parsed)}"
        )
        records = [parsed]
    else:
        assert isinstance(parsed, list), (
            f"Multi-record JSON should be an array, got {type(parsed)}"
        )
        records = parsed
        assert len(records) == expected_records, (
            f"Expected {expected_records} records, got {len(records)}"
        )

    for record in records:
        assert "id" in record, "Record missing 'id' field"
        assert "reqType" in record, "Record missing 'reqType' field"
        assert record["reqType"] in ("request", "response"), (
            f"Unexpected reqType: {record['reqType']}"
        )
        assert "contentType" in record, "Record missing 'contentType' field"

    return records


def verify_csv_object(data: bytes, expected_records: int):
    """Parse CSV marshaller output and validate structure.

    CSV output has a header row matching logRecord field names, followed
    by data rows. The bytes field is base64-encoded.
    """
    text = data.decode("utf-8")
    reader = csv.reader(io.StringIO(text))
    rows = list(reader)

    assert len(rows) >= 1, "CSV has no rows"
    header = rows[0]
    assert header == LOG_RECORD_COLUMNS, (
        f"CSV header mismatch.\nExpected: {LOG_RECORD_COLUMNS}\nGot: {header}"
    )

    data_rows = rows[1:]
    assert len(data_rows) == expected_records, (
        f"Expected {expected_records} data rows, got {len(data_rows)}"
    )

    for row in data_rows:
        assert len(row) == len(LOG_RECORD_COLUMNS), (
            f"Row has {len(row)} fields, expected {len(LOG_RECORD_COLUMNS)}"
        )
        # reqType is column index 3
        assert row[3] in ("request", "response"), (
            f"Unexpected reqType: {row[3]}"
        )
        # bytes is column index 1 — must be valid base64
        if row[1]:
            base64.b64decode(row[1])

    return data_rows


def verify_parquet_object(data: bytes, expected_records: int):
    """Parse Parquet marshaller output and validate structure.

    Parquet schema columns must match logRecord field names.
    The bytes field is base64-encoded.
    """
    table = pq.read_table(io.BytesIO(data))

    schema_names = table.schema.names
    assert schema_names == LOG_RECORD_COLUMNS, (
        f"Parquet schema mismatch.\nExpected: {LOG_RECORD_COLUMNS}\nGot: {schema_names}"
    )

    assert table.num_rows == expected_records, (
        f"Expected {expected_records} rows, got {table.num_rows}"
    )

    req_types = table.column("reqType").to_pylist()
    for rt in req_types:
        assert rt in ("request", "response"), (
            f"Unexpected reqType: {rt}"
        )

    # Verify bytes column contains valid base64
    bytes_col = table.column("bytes").to_pylist()
    for b in bytes_col:
        if b:
            base64.b64decode(b)

    return table
