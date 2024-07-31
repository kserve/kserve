import unittest.mock as mock

from kserve.storage import Storage


def test_download_hf():
    uri = "hf://example.com/model:hash_value"

    mock_tokenizer_instance = mock.MagicMock()
    patch_tokenizer = mock.patch(
        "transformers.AutoTokenizer.from_pretrained",
        return_value=mock_tokenizer_instance,
    )

    mock_config_instance = mock.MagicMock()
    patch_config = mock.patch(
        "transformers.AutoConfig.from_pretrained", return_value=mock_config_instance
    )

    mock_model_instance = mock.MagicMock()
    patch_model = mock.patch(
        "transformers.AutoModel.from_config", return_value=mock_model_instance
    )

    with patch_tokenizer, patch_config, patch_model:
        Storage.download(uri)

    mock_tokenizer_instance.save_pretrained.assert_called_once()
    mock_model_instance.save_pretrained.assert_called_once()
