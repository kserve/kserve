/* Copyright 2017 The TensorFlow Authors. All Rights Reserved.

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
#include "tensorflow/core/framework/dataset.h"
#include "tensorflow/core/framework/partial_tensor_shape.h"
#include "tensorflow/core/framework/tensor.h"

namespace tensorflow {
namespace data {
namespace {

// See documentation in ../../ops/dataset_ops.cc for a high-level
// description of the following op.

class TakeDatasetOp : public UnaryDatasetOpKernel {
 public:
  explicit TakeDatasetOp(OpKernelConstruction* ctx)
      : UnaryDatasetOpKernel(ctx) {}

 protected:
  void MakeDataset(OpKernelContext* ctx, DatasetBase* input,
                   DatasetBase** output) override {
    // Create a new TakeDatasetOp::Dataset, and return it as the output.
    int64 count;
    OP_REQUIRES_OK(ctx, ParseScalarArgument<int64>(ctx, "count", &count));
    *output = new Dataset(ctx, count, input);
  }

 private:
  class Dataset : public DatasetBase {
   public:
    Dataset(OpKernelContext* ctx, int64 count, const DatasetBase* input)
        : DatasetBase(DatasetContext(ctx)), count_(count), input_(input) {
      input_->Ref();
    }

    ~Dataset() override { input_->Unref(); }

    std::unique_ptr<IteratorBase> MakeIteratorInternal(
        const string& prefix) const override {
      if (count_ == 0) {
        return std::unique_ptr<IteratorBase>(
            new EmptyIterator({this, strings::StrCat(prefix, "::EmptyTake")}));
      } else {
        return std::unique_ptr<IteratorBase>(new FiniteIterator(
            {this, strings::StrCat(prefix, "::FiniteTake")}));
      }
    }

    const DataTypeVector& output_dtypes() const override {
      return input_->output_dtypes();
    }

    const std::vector<PartialTensorShape>& output_shapes() const override {
      return input_->output_shapes();
    }

    string DebugString() const override { return "TakeDatasetOp::Dataset"; }

    int64 Cardinality() const override {
      int64 n = input_->Cardinality();
      if (n == kUnknownCardinality) {
        return kUnknownCardinality;
      }
      if (n == kInfiniteCardinality) {
        return count_;
      }
      return std::min(n, count_);
    }

   protected:
    Status AsGraphDefInternal(SerializationContext* ctx,
                              DatasetGraphDefBuilder* b,
                              Node** output) const override {
      Node* input_graph_node = nullptr;
      TF_RETURN_IF_ERROR(b->AddInputDataset(ctx, input_, &input_graph_node));
      Node* count = nullptr;
      TF_RETURN_IF_ERROR(b->AddScalar(count_, &count));
      TF_RETURN_IF_ERROR(
          b->AddDataset(this, {input_graph_node, count}, output));
      return Status::OK();
    }

   private:
    class EmptyIterator : public DatasetIterator<Dataset> {
     public:
      explicit EmptyIterator(const Params& params)
          : DatasetIterator<Dataset>(params) {}
      Status GetNextInternal(IteratorContext* ctx,
                             std::vector<Tensor>* out_tensors,
                             bool* end_of_sequence) override {
        *end_of_sequence = true;
        return Status::OK();
      }

     protected:
      std::shared_ptr<model::Node> CreateNode(
          IteratorContext* ctx, model::Node::Args args) const override {
        return model::MakeKnownRatioNode(std::move(args),
                                         /*ratio=*/1);
      }

      Status SaveInternal(IteratorStateWriter* writer) override {
        return Status::OK();
      }

      Status RestoreInternal(IteratorContext* ctx,
                             IteratorStateReader* reader) override {
        return Status::OK();
      }
    };

    class FiniteIterator : public DatasetIterator<Dataset> {
     public:
      explicit FiniteIterator(const Params& params)
          : DatasetIterator<Dataset>(params), i_(0) {}

      Status Initialize(IteratorContext* ctx) override {
        return dataset()->input_->MakeIterator(ctx, prefix(), &input_impl_);
      }

      Status GetNextInternal(IteratorContext* ctx,
                             std::vector<Tensor>* out_tensors,
                             bool* end_of_sequence) override {
        mutex_lock l(mu_);  // TODO(mrry): Make locking less conservative.
        if (!input_impl_) {
          *end_of_sequence = true;
          return Status::OK();
        }
        while (dataset()->count_ < 0 || i_ < dataset()->count_) {
          TF_RETURN_IF_ERROR(
              input_impl_->GetNext(ctx, out_tensors, end_of_sequence));
          if (!*end_of_sequence) {
            ++i_;
            return Status::OK();
          }
          break;
        }
        *end_of_sequence = true;
        input_impl_.reset();
        return Status::OK();
      }

     protected:
      std::shared_ptr<model::Node> CreateNode(
          IteratorContext* ctx, model::Node::Args args) const override {
        return model::MakeKnownRatioNode(std::move(args),
                                         /*ratio=*/1);
      }

      Status SaveInternal(IteratorStateWriter* writer) override {
        mutex_lock l(mu_);
        TF_RETURN_IF_ERROR(writer->WriteScalar(full_name("i"), i_));
        if (input_impl_) {
          TF_RETURN_IF_ERROR(SaveInput(writer, input_impl_));
        } else {
          TF_RETURN_IF_ERROR(
              writer->WriteScalar(full_name("input_impl_empty"), ""));
        }
        return Status::OK();
      }

      Status RestoreInternal(IteratorContext* ctx,
                             IteratorStateReader* reader) override {
        mutex_lock l(mu_);
        TF_RETURN_IF_ERROR(reader->ReadScalar(full_name("i"), &i_));
        if (!reader->Contains(full_name("input_impl_empty"))) {
          TF_RETURN_IF_ERROR(RestoreInput(ctx, reader, input_impl_));
        } else {
          input_impl_.reset();
        }
        return Status::OK();
      }

     private:
      mutex mu_;
      int64 i_ GUARDED_BY(mu_);
      std::unique_ptr<IteratorBase> input_impl_ GUARDED_BY(mu_);
    };

    const int64 count_;
    const DatasetBase* const input_;
  };
};

REGISTER_KERNEL_BUILDER(Name("TakeDataset").Device(DEVICE_CPU), TakeDatasetOp);

}  // namespace
}  // namespace data
}  // namespace tensorflow
