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

#include <deque>
#include <vector>

#include "tensorflow/core/framework/dataset.h"
#include "tensorflow/core/framework/partial_tensor_shape.h"
#include "tensorflow/core/framework/tensor.h"
#include "tensorflow/core/platform/logging.h"
#include "tensorflow/core/util/batch_util.h"

namespace tensorflow {
namespace data {
namespace {

// See documentation in ../../ops/dataset_ops.cc for a high-level
// description of the following op.

class SlidingWindowDatasetOp : public UnaryDatasetOpKernel {
 public:
  explicit SlidingWindowDatasetOp(OpKernelConstruction* ctx)
      : UnaryDatasetOpKernel(ctx) {}

  void MakeDataset(OpKernelContext* ctx, DatasetBase* input,
                   DatasetBase** output) override {
    int64 window_size = 0;
    OP_REQUIRES_OK(
        ctx, ParseScalarArgument<int64>(ctx, "window_size", &window_size));
    OP_REQUIRES(
        ctx, window_size > 0,
        errors::InvalidArgument("Window size must be greater than zero."));
    int64 window_shift = 0;
    OP_REQUIRES_OK(
        ctx, ParseScalarArgument<int64>(ctx, "window_shift", &window_shift));
    OP_REQUIRES(
        ctx, window_shift > 0,
        errors::InvalidArgument("Window shift must be greater than zero."));
    int64 window_stride = 0;
    OP_REQUIRES_OK(
        ctx, ParseScalarArgument<int64>(ctx, "window_stride", &window_stride));
    OP_REQUIRES(
        ctx, window_stride > 0,
        errors::InvalidArgument("window_stride must be greater than zero."));
    if (window_size == window_shift && window_stride == 1) {
      LOG(WARNING) << "window_shift: " << window_shift
                   << " is equal to window_size: " << window_size
                   << " and window_stride is 1, use `batch` instead.";
    }
    *output = new Dataset(ctx, window_size, window_shift, window_stride, input);
  }

 private:
  class Dataset : public DatasetBase {
   public:
    Dataset(OpKernelContext* ctx, int64 window_size, int64 window_shift,
            int64 window_stride, const DatasetBase* input)
        : DatasetBase(DatasetContext(ctx)),
          window_size_(window_size),
          window_shift_(window_shift),
          window_stride_(window_stride),
          input_(input) {
      input_->Ref();

      const auto& input_shapes = input_->output_shapes();
      output_shapes_.reserve(input_shapes.size());
      for (const auto& input_shape : input_shapes) {
        output_shapes_.emplace_back(
            PartialTensorShape({-1}).Concatenate(input_shape));
      }
    }

    ~Dataset() override { input_->Unref(); }

    std::unique_ptr<IteratorBase> MakeIteratorInternal(
        const string& prefix) const override {
      return std::unique_ptr<IteratorBase>(new Iterator(
          Iterator::Params{this, strings::StrCat(prefix, "::Slide")}));
    }

    const DataTypeVector& output_dtypes() const override {
      return input_->output_dtypes();
    }

    const std::vector<PartialTensorShape>& output_shapes() const override {
      return output_shapes_;
    }

    string DebugString() const override {
      return strings::StrCat("SlidingWindowDatasetOp(", window_size_, ", ",
                             window_shift_, ", ", window_stride_, ")::Dataset");
    }

    int64 Cardinality() const override {
      int64 n = input_->Cardinality();
      if (n == kInfiniteCardinality || n == kUnknownCardinality) {
        return n;
      }
      return n / window_shift_;
    }

   protected:
    Status AsGraphDefInternal(SerializationContext* ctx,
                              DatasetGraphDefBuilder* b,
                              Node** output) const override {
      Node* input_graph_node = nullptr;
      TF_RETURN_IF_ERROR(b->AddInputDataset(ctx, input_, &input_graph_node));
      Node* window_size = nullptr;
      Node* window_shift = nullptr;
      Node* window_stride = nullptr;
      TF_RETURN_IF_ERROR(b->AddScalar(window_size_, &window_size));
      TF_RETURN_IF_ERROR(b->AddScalar(window_shift_, &window_shift));
      TF_RETURN_IF_ERROR(b->AddScalar(window_stride_, &window_stride));
      TF_RETURN_IF_ERROR(b->AddDataset(
          this, {input_graph_node, window_size, window_shift, window_stride},
          output));
      return Status::OK();
    }

   private:
    class Iterator : public DatasetIterator<Dataset> {
     public:
      explicit Iterator(const Params& params)
          : DatasetIterator<Dataset>(params) {}

      Status Initialize(IteratorContext* ctx) override {
        return dataset()->input_->MakeIterator(ctx, prefix(), &input_impl_);
      }

      Status GetNextInternal(IteratorContext* ctx,
                             std::vector<Tensor>* out_tensors,
                             bool* end_of_sequence) override {
        const int64 window_size = dataset()->window_size_;
        const int64 window_shift = dataset()->window_shift_;
        const int64 window_stride = dataset()->window_stride_;
        std::vector<std::vector<Tensor>> batch_elements;
        {
          mutex_lock l(mu_);
          if (!input_impl_) {
            *end_of_sequence = true;
            return Status::OK();
          }
          batch_elements.reserve(window_size);

          // Fill up buffer.
          size_t target_size = TargetBufferSize(window_size, window_stride);
          *end_of_sequence = false;
          for (size_t i = buffer_.size(); i < target_size && !*end_of_sequence;
               ++i) {
            std::vector<Tensor> element;
            TF_RETURN_IF_ERROR(
                input_impl_->GetNext(ctx, &element, end_of_sequence));
            if (!*end_of_sequence) {
              buffer_.push_back(std::move(element));
            } else {
              input_impl_.reset();
            }
          }

          // Drop the final smaller batch.
          if (buffer_.size() < target_size) {
            DCHECK(*end_of_sequence);
            return Status::OK();
          }

          for (size_t i = 0; i < window_size; ++i) {
            batch_elements.emplace_back(buffer_[window_stride * i]);
          }

          // Drop the data before the next iteration.
          if (window_shift >= buffer_.size()) {
            for (size_t i = buffer_.size(); i < window_shift; ++i) {
              bool end_of_input;
              std::vector<Tensor> element;
              TF_RETURN_IF_ERROR(
                  input_impl_->GetNext(ctx, &element, &end_of_input));
              if (end_of_input) {
                input_impl_.reset();
                break;
              }
            }
            buffer_.clear();
          } else {
            buffer_.erase(buffer_.begin(), buffer_.begin() + window_shift);
          }
        }

        // Construct output tensors.
        const size_t num_tuple_components = batch_elements[0].size();
        const int64 num_batch_elements = batch_elements.size();
        for (size_t component_index = 0; component_index < num_tuple_components;
             ++component_index) {
          const Tensor& first_element = batch_elements[0][component_index];
          TensorShape batch_component_shape({num_batch_elements});
          batch_component_shape.AppendShape(first_element.shape());
          out_tensors->emplace_back(ctx->allocator({}), first_element.dtype(),
                                    batch_component_shape);
          Tensor& batch_component = out_tensors->back();
          // Build the output tuple component by copying one slice
          // from each input element in the batch.
          for (size_t i = 0; i < num_batch_elements; ++i) {
            if (batch_elements[i][component_index].shape() !=
                first_element.shape()) {
              return errors::InvalidArgument(
                  "Cannot batch tensors with different shapes in component ",
                  component_index, ". First element had shape ",
                  first_element.shape().DebugString(), " and element ", i,
                  " had shape ",
                  batch_elements[i][component_index].shape().DebugString(),
                  ".");
            }
            TF_RETURN_IF_ERROR(batch_util::CopyElementToSlice(
                std::move(batch_elements[i][component_index]), &batch_component,
                i));
          }
        }
        *end_of_sequence = false;
        return Status::OK();
      }

     protected:
      std::shared_ptr<model::Node> CreateNode(
          IteratorContext* ctx, model::Node::Args args) const override {
        return model::MakeKnownRatioNode(std::move(args),
                                         dataset()->window_shift_);
      }

      Status SaveInternal(IteratorStateWriter* writer) override {
        mutex_lock l(mu_);
        if (!input_impl_) {
          TF_RETURN_IF_ERROR(
              writer->WriteScalar(full_name("input_impl_empty"), ""));
        } else {
          TF_RETURN_IF_ERROR(SaveInput(writer, input_impl_));
        }
        // Save buffer.
        TF_RETURN_IF_ERROR(writer->WriteScalar(strings::StrCat("buffer_size"),
                                               buffer_.size()));
        for (int64 i = 0; i < buffer_.size(); i++) {
          TF_RETURN_IF_ERROR(writer->WriteScalar(
              strings::StrCat("buffer[", i, "]_size"), buffer_[i].size()));
          for (int64 j = 0; j < buffer_[i].size(); j++) {
            TF_RETURN_IF_ERROR(writer->WriteTensor(
                strings::StrCat("buffer[", i, "][", j, "]"), buffer_[i][j]));
          }
        }
        return Status::OK();
      }

      Status RestoreInternal(IteratorContext* ctx,
                             IteratorStateReader* reader) override {
        mutex_lock l(mu_);
        if (!reader->Contains(full_name("input_impl_empty"))) {
          TF_RETURN_IF_ERROR(RestoreInput(ctx, reader, input_impl_));
        } else {
          input_impl_.reset();
        }
        // Restore buffer.
        int64 buffer_size;
        TF_RETURN_IF_ERROR(
            reader->ReadScalar(strings::StrCat("buffer_size"), &buffer_size));
        buffer_.resize(buffer_size);
        for (int64 i = 0; i < buffer_size; i++) {
          int64 vector_size;
          TF_RETURN_IF_ERROR(reader->ReadScalar(
              strings::StrCat("buffer[", i, "]_size"), &vector_size));
          buffer_[i].resize(vector_size);
          for (int64 j = 0; j < vector_size; j++) {
            TF_RETURN_IF_ERROR(reader->ReadTensor(
                strings::StrCat("buffer[", i, "][", j, "]"), &buffer_[i][j]));
          }
        }
        return Status::OK();
      }

     private:
      size_t TargetBufferSize(int64 window_size, int64 window_stride) {
        return (window_size - 1) * window_stride + 1;
      }

      mutex mu_;
      std::deque<std::vector<Tensor>> buffer_ GUARDED_BY(mu_);
      std::unique_ptr<IteratorBase> input_impl_ GUARDED_BY(mu_);
    };

    const int64 window_size_;
    const int64 window_shift_;
    const int64 window_stride_;
    const DatasetBase* const input_;
    std::vector<PartialTensorShape> output_shapes_;
  };
};

REGISTER_KERNEL_BUILDER(
    Name("ExperimentalSlidingWindowDataset").Device(DEVICE_CPU),
    SlidingWindowDatasetOp);

}  // namespace
}  // namespace data
}  // namespace tensorflow
