# Copyright 2016 The TensorFlow Authors. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ==============================================================================

"""Implements a number of text preprocessing utilities (deprecated).

This module and all its submodules are deprecated. See
[contrib/learn/README.md](https://www.tensorflow.org/code/tensorflow/contrib/learn/README.md)
for migration instructions.
"""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import re
import numpy as np
import six

from tensorflow.python.platform import gfile
from tensorflow.python.util.deprecation import deprecated

from .categorical_vocabulary import CategoricalVocabulary  # pylint: disable=g-bad-import-order

try:
  # pylint: disable=g-import-not-at-top
  import cPickle as pickle
except ImportError:
  # pylint: disable=g-import-not-at-top
  import pickle

TOKENIZER_RE = re.compile(r"[A-Z]{2,}(?![a-z])|[A-Z][a-z]+(?=[A-Z])|[\'\w\-]+",
                          re.UNICODE)


@deprecated(None, 'Please use tensorflow/transform or tf.data.')
def tokenizer(iterator):
  """Tokenizer generator.

  Args:
    iterator: Input iterator with strings.

  Yields:
    array of tokens per each value in the input.
  """
  for value in iterator:
    yield TOKENIZER_RE.findall(value)


@deprecated(None, 'Please use tensorflow/transform or tf.data.')
class ByteProcessor(object):
  """Maps documents into sequence of ids for bytes.

  THIS CLASS IS DEPRECATED. See
  [contrib/learn/README.md](https://www.tensorflow.org/code/tensorflow/contrib/learn/README.md)
  for general migration instructions.
  """

  @deprecated(None, 'Please use tensorflow/transform or tf.data.')
  def __init__(self, max_document_length):
    self.max_document_length = max_document_length

  def fit(self, x):
    """Does nothing. No fitting required."""
    pass

  def fit_transform(self, x):
    """Calls transform."""
    return self.transform(x)

  # pylint: disable=no-self-use
  def reverse(self, x):
    """Reverses output of transform back to text.

    Args:
      x: iterator or matrix of integers. Document representation in bytes.

    Yields:
      Iterators of utf-8 strings.
    """
    for data in x:
      document = np.trim_zeros(data.astype(np.int8), trim='b').tostring()
      try:
        yield document.decode('utf-8')
      except UnicodeDecodeError:
        yield ''

  def transform(self, x):
    """Transforms input documents into sequence of ids.

    Args:
      x: iterator or list of input documents.
        Documents can be bytes or unicode strings, which will be encoded as
        utf-8 to map to bytes. Note, in Python2 str and bytes is the same type.

    Yields:
      iterator of byte ids.
    """
    if six.PY3:
      # For Python3 defined buffer as memoryview.
      buffer_or_memoryview = memoryview
    else:
      buffer_or_memoryview = buffer  # pylint: disable=undefined-variable
    for document in x:
      if isinstance(document, six.text_type):
        document = document.encode('utf-8')
      document_mv = buffer_or_memoryview(document)
      buff = np.frombuffer(document_mv[:self.max_document_length],
                           dtype=np.uint8)
      yield np.pad(buff, (0, self.max_document_length - len(buff)), 'constant')


class VocabularyProcessor(object):
  """Maps documents to sequences of word ids.

  THIS CLASS IS DEPRECATED. See
  [contrib/learn/README.md](https://www.tensorflow.org/code/tensorflow/contrib/learn/README.md)
  for general migration instructions.
  """

  @deprecated(None, 'Please use tensorflow/transform or tf.data.')
  def __init__(self,
               max_document_length,
               min_frequency=0,
               vocabulary=None,
               tokenizer_fn=None):
    """Initializes a VocabularyProcessor instance.

    Args:
      max_document_length: Maximum length of documents.
        if documents are longer, they will be trimmed, if shorter - padded.
      min_frequency: Minimum frequency of words in the vocabulary.
      vocabulary: CategoricalVocabulary object.

    Attributes:
      vocabulary_: CategoricalVocabulary object.
    """
    self.max_document_length = max_document_length
    self.min_frequency = min_frequency
    if vocabulary:
      self.vocabulary_ = vocabulary
    else:
      self.vocabulary_ = CategoricalVocabulary()
    if tokenizer_fn:
      self._tokenizer = tokenizer_fn
    else:
      self._tokenizer = tokenizer

  def fit(self, raw_documents, unused_y=None):
    """Learn a vocabulary dictionary of all tokens in the raw documents.

    Args:
      raw_documents: An iterable which yield either str or unicode.
      unused_y: to match fit format signature of estimators.

    Returns:
      self
    """
    for tokens in self._tokenizer(raw_documents):
      for token in tokens:
        self.vocabulary_.add(token)
    if self.min_frequency > 0:
      self.vocabulary_.trim(self.min_frequency)
    self.vocabulary_.freeze()
    return self

  def fit_transform(self, raw_documents, unused_y=None):
    """Learn the vocabulary dictionary and return indexies of words.

    Args:
      raw_documents: An iterable which yield either str or unicode.
      unused_y: to match fit_transform signature of estimators.

    Returns:
      x: iterable, [n_samples, max_document_length]. Word-id matrix.
    """
    self.fit(raw_documents)
    return self.transform(raw_documents)

  def transform(self, raw_documents):
    """Transform documents to word-id matrix.

    Convert words to ids with vocabulary fitted with fit or the one
    provided in the constructor.

    Args:
      raw_documents: An iterable which yield either str or unicode.

    Yields:
      x: iterable, [n_samples, max_document_length]. Word-id matrix.
    """
    for tokens in self._tokenizer(raw_documents):
      word_ids = np.zeros(self.max_document_length, np.int64)
      for idx, token in enumerate(tokens):
        if idx >= self.max_document_length:
          break
        word_ids[idx] = self.vocabulary_.get(token)
      yield word_ids

  def reverse(self, documents):
    """Reverses output of vocabulary mapping to words.

    Args:
      documents: iterable, list of class ids.

    Yields:
      Iterator over mapped in words documents.
    """
    for item in documents:
      output = []
      for class_id in item:
        output.append(self.vocabulary_.reverse(class_id))
      yield ' '.join(output)

  def save(self, filename):
    """Saves vocabulary processor into given file.

    Args:
      filename: Path to output file.
    """
    with gfile.Open(filename, 'wb') as f:
      f.write(pickle.dumps(self))

  @classmethod
  def restore(cls, filename):
    """Restores vocabulary processor from given file.

    Args:
      filename: Path to file to load from.

    Returns:
      VocabularyProcessor object.
    """
    with gfile.Open(filename, 'rb') as f:
      return pickle.loads(f.read())
