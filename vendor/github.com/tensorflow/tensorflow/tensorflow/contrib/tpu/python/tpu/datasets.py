# Copyright 2017 The TensorFlow Authors. All Rights Reserved.
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
# ======================================
"""Library of Cloud TPU helper functions for data loading."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.python.data.experimental.ops import batching
from tensorflow.python.data.experimental.ops import interleave_ops
from tensorflow.python.data.ops import dataset_ops
from tensorflow.python.data.ops import iterator_ops
from tensorflow.python.data.ops import readers
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import function
from tensorflow.python.framework import ops
from tensorflow.python.ops import functional_ops


def _TextLineDataset(filename):
  buffer_size = 8 * 1024 * 1024  # 8 MiB per file
  dataset = readers.TextLineDataset(filename, buffer_size=buffer_size)
  return dataset


def _TFRecordDataset(filename):
  buffer_size = 8 * 1024 * 1024  # 8 MiB per file
  dataset = readers.TFRecordDataset(filename, buffer_size=buffer_size)
  return dataset


_FILETYPE_MAP = {
    'tfrecord': _TFRecordDataset,
    'textline': _TextLineDataset,
    'text': _TextLineDataset,
}


def StreamingFilesDataset(files,
                          filetype=None,
                          file_reader_job=None,
                          worker_job=None,
                          num_epochs=None,
                          filename_shuffle_buffer_size=None,
                          num_parallel_reads=None,
                          batch_transfer_size=None,
                          sloppy=None):
  """StreamingFilesDataset constructs a dataset to stream from workers (GCE VM).

  Because Cloud TPUs are allocated over the network, a Cloud TPU cannot read
  files local to your GCE VM. In order to train using files stored on your local
  VM (e.g. on local SSD for extreme performance), use the StreamingFilesDataset
  helper to generate a dataset to feed your Cloud TPU with files from your GCE
  VM.

  The resulting dataset may return an OutOfRangeError if there are no files
  found as a result of the fileglob expansion.

  Note: StreamingFilesDataset assumes that the session is using a
  TPUClusterResolver and has therefore a worker and a coordinator job. File
  loading will be done on the coordinator job.

  Args:
    files: A string glob to match files, or a `tf.data.Dataset` generating file
      names.
    filetype: A string (one of 'tfrecord', or 'textline') or a single-argument
      TensorFlow function that when given a filename returns a dataset.
    file_reader_job: An optional string that corresponds to the job that should
      perform the file reads.
    worker_job: An optional string that corresponds to the job that should
      process the tensors (i.e. your GPU or TPU worker).
    num_epochs: The number of epochs through the training set that should be
      generated. By default, it will repeat infinitely.
    filename_shuffle_buffer_size: An optional integer whose value controls the
      shuffling of the file names. If you would like to read from the files in
      the same order, set to 0 or False.
    num_parallel_reads: An optional integer controlling the number of files to
      read from concurrently. (Set to 1 for no parallelism.)
    batch_transfer_size: An optional integer controlling the batching used to
      amortize the remote function invocation overhead. Set to a very large
      number to increase throughput. Set to a very small number to reduce memory
      consumption. Set to False to skip batching.
    sloppy: (Optional.) If `False`, read input data while maintaining a
      deterministic order. (This may have significant performance impacts.)
      sloppy defaults to: True.
  Returns:
    A `tf.data.Dataset` with an infinite stream of elements generated by a
    parallel interleaving of the set of files matched (or generated) by `files`
    with a type is the output of the dataset specified by `filetype`.

  Raises:
    ValueError: if any argument is not of the expected type.
  """
  if filetype is None:
    filetype = 'tfrecord'

  if isinstance(filetype, str):
    if filetype not in _FILETYPE_MAP:
      raise ValueError('Unexpected filetype: %s' % filetype)
    reader_fn = _FILETYPE_MAP[filetype]
  elif callable(filetype):
    reader_fn = filetype
  else:
    raise ValueError('filetype should be a string or a callable')

  file_reader_job = file_reader_job or 'coordinator'

  worker_job = worker_job or 'worker'

  if filename_shuffle_buffer_size is None:
    filename_shuffle_buffer_size = 4096

  num_parallel_reads = num_parallel_reads or 8

  if batch_transfer_size is None:
    batch_transfer_size = 256

  if sloppy is None:
    sloppy = True

  with ops.device('/job:%s' % file_reader_job):
    if isinstance(files, str):
      source_dataset = dataset_ops.Dataset.list_files(files)
    elif isinstance(files, dataset_ops.DatasetV2):
      source_dataset = files
    else:
      raise ValueError('files was not a string or a dataset: %s' % files)

    if filename_shuffle_buffer_size:
      source_dataset = source_dataset.shuffle(
          buffer_size=filename_shuffle_buffer_size)

    # NOTE: We perform the `repeat` on the source dataset, because the output
    # dataset does not currently have enough information to recreate an iterator
    # over the source dataset when it reaches the end.
    source_dataset = source_dataset.repeat(num_epochs)

    source_dataset = source_dataset.apply(
        interleave_ops.parallel_interleave(
            reader_fn, cycle_length=num_parallel_reads, sloppy=sloppy))

    if batch_transfer_size:
      source_dataset = source_dataset.batch(batch_transfer_size)

    source_dataset = source_dataset.prefetch(1)

    source_iterator = dataset_ops.make_one_shot_iterator(source_dataset)
    source_handle = source_iterator.string_handle()

  @function.Defun(dtypes.string)
  def LoadingFunc(h):
    remote_iterator = iterator_ops.Iterator.from_string_handle(
        h, source_dataset.output_types, source_dataset.output_shapes)
    return remote_iterator.get_next()

  def MapFn(unused_input):
    if isinstance(source_dataset.output_types, dtypes.DType):
      output_types = [source_dataset.output_types]
    elif isinstance(source_dataset.output_types, (list, tuple)):
      output_types = source_dataset.output_types
    else:
      raise ValueError('source dataset has invalid output types')
    remote_calls = functional_ops.remote_call(
        args=[source_handle],
        Tout=output_types,
        f=LoadingFunc,
        target='/job:%s/replica:0/task:0/cpu:0' % file_reader_job)
    if len(remote_calls) == 1:
      return remote_calls[0]
    else:
      return remote_calls

  with ops.device('/job:%s' % worker_job):
    output_dataset = dataset_ops.Dataset.range(2).repeat().map(
        MapFn, num_parallel_calls=4 if sloppy else None)
    output_dataset = output_dataset.prefetch(1)

    if batch_transfer_size:
      # Undo the batching used during the transfer.
      output_dataset = output_dataset.apply(batching.unbatch()).prefetch(1)

  return output_dataset
