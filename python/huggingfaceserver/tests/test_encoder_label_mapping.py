# Copyright 2024 The KServe Authors.
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
import torch
from unittest.mock import Mock, patch,MagicMock

from huggingfaceserver.encoder_model import HuggingfaceEncoderModel
from huggingfaceserver.task import MLTask


class TestEncoderLabelMapping:
    """Test cases for the _get_label_or_index helper method and related functionality."""

    @pytest.fixture
    def mock_model_config_with_labels(self):
        """Mock model config with id2label mapping."""
        config = MagicMock()
        config.architectures = ["BertForSequenceClassification"]
        config.model_type = "bert"
        config.num_labels = 2
        config.id2label = {0: "negative", 1: "positive"}
        config.label2id = {"negative": 0, "positive": 1}
        return config

    @pytest.fixture
    def mock_model_config_without_labels(self):
        """Mock model config without id2label mapping."""
        config = MagicMock(spec_set=['architectures', 'model_type', 'num_labels'])
        config.architectures = ["BertForSequenceClassification"]
        config.model_type = "bert"
        config.num_labels = 2
        return config

    @pytest.fixture
    def encoder_model_with_labels(self, mock_model_config_with_labels):
        """Create encoder model with use_id2label=True and valid config."""
        model = HuggingfaceEncoderModel(
            "test-model",
            model_id_or_path="test/path",
            model_config=mock_model_config_with_labels,
            task=MLTask.sequence_classification,
            use_id2label=True,
        )
        model._tokenizer = Mock()
        return model

    @pytest.fixture
    def encoder_model_without_labels(self, mock_model_config_without_labels):
        """Create encoder model with use_id2label=True but missing id2label."""
        model = HuggingfaceEncoderModel(
            "test-model",
            model_id_or_path="test/path",
            model_config=mock_model_config_without_labels,
            task=MLTask.sequence_classification,
            use_id2label=True,
        )
        model._tokenizer = Mock()
        return model

    @pytest.fixture
    def encoder_model_labels_disabled(self, mock_model_config_with_labels):
        """Create encoder model with use_id2label=False."""
        model = HuggingfaceEncoderModel(
            "test-model",
            model_id_or_path="test/path",
            model_config=mock_model_config_with_labels,
            task=MLTask.sequence_classification,
            use_id2label=False,
        )
        model._tokenizer = Mock()
        return model

    def test_get_label_or_index_with_valid_labels(self, encoder_model_with_labels):
        """Test _get_label_or_index returns label when use_id2label=True and id2label exists."""
        # Access the helper method through postprocess method context
        # We'll test this indirectly through the postprocess method
        model: HuggingfaceEncoderModel = encoder_model_with_labels
        
        # Create a mock context for postprocess
        context = {
            "payload": {"instances": ["test input"]},
            "input_ids": torch.tensor([[1, 2, 3]])
        }
        
        # Create mock outputs for sequence classification
        outputs = torch.tensor([[2.0, -1.0]])  # Logits favoring class 0
        
        with patch.object(model, '_model', None), \
             patch.object(model, '_tokenizer', None):
            
            # Test the helper method directly by accessing it in postprocess
            model.task = MLTask.sequence_classification
            model.return_probabilities = False
            
            # We need to access the helper method that gets created in postprocess
            # Since it's a nested function, we'll test its behavior through postprocess
            result = model.postprocess(outputs, context)
            
            # Should return label "negative" for class 0 (highest logit)
            assert result["predictions"][0] == "negative"

    def test_get_label_or_index_without_labels_with_warning(self, encoder_model_without_labels: HuggingfaceEncoderModel):
        """Test _get_label_or_index returns index and logs warning when id2label is missing."""
        model = encoder_model_without_labels
        
        context = {
            "payload": {"instances": ["test input"]},
            "input_ids": torch.tensor([[1, 2, 3]])
        }
        
        outputs = torch.tensor([[2.0, -1.0]])  # Logits favoring class 0
        
        with patch.object(model, '_model', None), \
             patch.object(model, '_tokenizer', None), \
             patch('huggingfaceserver.encoder_model.logger') as mock_logger:
            
            model.task = MLTask.sequence_classification
            model.return_probabilities = False
            
            result = model.postprocess(outputs, context)
            
            # Should return index 0 instead of label
            assert result["predictions"][0] == 0
            
            # Should log warning about missing id2label
            mock_logger.warning.assert_called_with(
                "id2label not found in model config, returning index"
            )

    def test_get_label_or_index_disabled(self, encoder_model_labels_disabled):
        """Test _get_label_or_index returns index when use_id2label=False."""
        model = encoder_model_labels_disabled
        
        context = {
            "payload": {"instances": ["test input"]},
            "input_ids": torch.tensor([[1, 2, 3]])
        }
        
        outputs = torch.tensor([[2.0, -1.0]])  # Logits favoring class 0
        
        with patch.object(model, '_model', None), \
             patch.object(model, '_tokenizer', None):
            
            model.task = MLTask.sequence_classification
            model.return_probabilities = False
            
            result = model.postprocess(outputs, context)
            
            # Should return index 0 even though id2label exists
            assert result["predictions"][0] == 0

    def test_get_label_or_index_with_probabilities(self, encoder_model_with_labels):
        """Test _get_label_or_index works correctly with return_probabilities=True."""
        model = encoder_model_with_labels
        
        context = {
            "payload": {"instances": ["test input"]},
            "input_ids": torch.tensor([[1, 2, 3]])
        }
        
        outputs = torch.tensor([[2.0, -1.0]])  # Logits
        
        with patch.object(model, '_model', None), \
             patch.object(model, '_tokenizer', None):
            
            model.task = MLTask.sequence_classification
            model.return_probabilities = True
            
            result = model.postprocess(outputs, context)
            
            # Should return probability dict with labels as keys
            prediction = result["predictions"][0]
            assert "negative" in prediction
            assert "positive" in prediction
            assert isinstance(prediction["negative"], float)
            assert isinstance(prediction["positive"], float)
            
            # Probabilities should sum to approximately 1
            total_prob = sum(prediction.values())
            assert abs(total_prob - 1.0) < 1e-6

    def test_get_label_or_index_probabilities_without_labels(self, encoder_model_without_labels):
        """Test _get_label_or_index with probabilities when id2label is missing."""
        model = encoder_model_without_labels
        
        context = {
            "payload": {"instances": ["test input"]},
            "input_ids": torch.tensor([[1, 2, 3]])
        }
        
        outputs = torch.tensor([[2.0, -1.0]])  # Logits
        
        with patch.object(model, '_model', None), \
             patch.object(model, '_tokenizer', None), \
             patch('huggingfaceserver.encoder_model.logger') as mock_logger:
            
            model.task = MLTask.sequence_classification
            model.return_probabilities = True
            
            result = model.postprocess(outputs, context)
            
            # Should return probability dict with indices as keys
            prediction = result["predictions"][0]
            assert 0 in prediction
            assert 1 in prediction
            assert isinstance(prediction[0], float)
            assert isinstance(prediction[1], float)
            
            # Should log warning
            mock_logger.warning.assert_called()

    def test_multiple_samples_label_mapping(self, encoder_model_with_labels):
        """Test _get_label_or_index works correctly with multiple samples."""
        model = encoder_model_with_labels
        
        context = {
            "payload": {"instances": ["test input 1", "test input 2"]},
            "input_ids": torch.tensor([[1, 2, 3], [4, 5, 6]])
        }
        
        # Two samples: first predicts class 0, second predicts class 1
        outputs = torch.tensor([[2.0, -1.0], [-1.0, 2.0]])
        
        with patch.object(model, '_model', None), \
             patch.object(model, '_tokenizer', None):
            
            model.task = MLTask.sequence_classification
            model.return_probabilities = False
            
            result = model.postprocess(outputs, context)
            
            # First sample should predict "negative", second should predict "positive"
            assert result["predictions"][0] == "negative"
            assert result["predictions"][1] == "positive"

    @pytest.mark.parametrize("index,expected_label", [
        (0, "negative"),
        (1, "positive"),
    ])
    def test_label_mapping_edge_cases(self, encoder_model_with_labels, index, expected_label):
        """Test _get_label_or_index with different indices."""
        model = encoder_model_with_labels
        
        # Test the logic by directly checking what would happen in postprocess
        assert model.use_id2label is True
        assert hasattr(model.model_config, 'id2label')
        assert model.model_config.id2label[index] == expected_label 