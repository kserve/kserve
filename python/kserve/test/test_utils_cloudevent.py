import json
from unittest import mock

import pytest

from kserve.utils.utils import create_response_cloudevent


@pytest.mark.parametrize("missing_attribute", ["time", "id"])
def test_create_response_cloudevent_merge_without_optional_attribute(
    missing_attribute,
):
    request_attributes = {
        "specversion": "1.0",
        "id": "request-id",
        "time": "request-time",
        "datacontenttype": "application/cloudevents+json",
    }
    request_attributes.pop(missing_attribute)
    request_attributes.update(
        {
            "source": "request-source",
            "type": "request-type",
            "custom-extension": "custom-value",
        }
    )
    original_attributes = request_attributes.copy()

    with mock.patch.dict("os.environ", {"CE_MERGE": "true"}):
        headers, body = create_response_cloudevent(
            "TestModel", {"predictions": []}, request_attributes, binary_event=True
        )

    assert json.loads(body) == {"predictions": []}
    assert headers["ce-custom-extension"] == "custom-value"
    assert headers["ce-id"] != "request-id"
    assert "ce-datacontenttype" not in headers
    assert request_attributes == original_attributes
