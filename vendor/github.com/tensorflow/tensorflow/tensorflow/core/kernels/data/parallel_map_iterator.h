/* Copyright 2018 The TensorFlow Authors. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
==============================================================================*/
#ifndef TENSORFLOW_CORE_KERNELS_DATA_PARALLEL_MAP_ITERATOR_H_
#define TENSORFLOW_CORE_KERNELS_DATA_PARALLEL_MAP_ITERATOR_H_

#include <memory>

#include "tensorflow/core/framework/dataset.h"

namespace tensorflow {
namespace data {

class ParallelMapFunctor {
 public:
  virtual ~ParallelMapFunctor() {}

  // A function that runs when the Iterator is initialized. It enables the user
  // to specify error checking logic that can fail early.
  virtual Status InitFunc(IteratorContext* ctx) { return Status::OK(); }

  // A function that transforms elements of one dataset into another
  // asynchronously. The arguments are:
  // 1. An `IteratorContext*` for the context in which the function should
  // execute.
  // 2. A `std::vector<Tensor>` containing the input element.
  // 3. A `std::vector<Tensor>*` to which the function will write the result.
  // 4. A `StatusCallback` that should be invoked when the function is complete.
  virtual void MapFunc(IteratorContext* ctx, const string& prefix,
                       std::vector<Tensor> input, std::vector<Tensor>* output,
                       StatusCallback callback) = 0;
};

// Returns a new iterator that uses `parallel_map_functor` to apply `MapFunc`
// to the elements of `input_dataset` using the given degree of parallelism.
std::unique_ptr<IteratorBase> NewParallelMapIterator(
    const DatasetBaseIterator::BaseParams& params,
    const DatasetBase* input_dataset,
    std::unique_ptr<ParallelMapFunctor> parallel_map_functor,
    int32 num_parallel_calls, bool sloppy, bool preserve_cardinality);

}  // namespace data
}  // namespace tensorflow

#endif  // TENSORFLOW_CORE_KERNELS_DATA_PARALLEL_MAP_ITERATOR_H_
