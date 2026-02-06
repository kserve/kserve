from kserve.metrics import get_labels


def test_get_labels():
    model_name = "sample-model"
    label = get_labels(model_name)
    assert label == {"model_name": model_name}
