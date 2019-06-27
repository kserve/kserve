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

#include "tensorflow/core/example/example.pb.h"
#include "tensorflow/core/example/feature.pb.h"
#include "tensorflow/core/framework/dataset.h"
#include "tensorflow/core/framework/partial_tensor_shape.h"
#include "tensorflow/core/framework/stats_aggregator.h"
#include "tensorflow/core/framework/tensor.h"
#include "tensorflow/core/lib/random/random.h"

namespace tensorflow {
namespace data {
namespace {

// This op defines a `Dataset` that passes through its input elements and
// records the latency of producing each element in the context's
// `StatsAggregator`.
//
// TODO(mrry): It is likely that many *StatsDatasetOp kernels will have the
// same or similar structure. We should abstract the common boilerplate into
// a base case and/or investigate how to make general-purpose *StatsDatasetOp
// kernels that use TensorFlow functions to represent their logic. For example,
// if the performance were adequate, we might replace this kernel with an
// implementation that executes functions before and after the `GetNext()` call
// on the input, each executing an op that gets the current time and performing
// the subtraction.
class LatencyStatsDatasetOp : public UnaryDatasetOpKernel {
 public:
  explicit LatencyStatsDatasetOp(OpKernelConstruction* ctx)
      : UnaryDatasetOpKernel(ctx) {}

  void MakeDataset(OpKernelContext* ctx, DatasetBase* input,
                   DatasetBase** output) override {
    string tag;
    OP_REQUIRES_OK(ctx, ParseScalarArgument(ctx, "tag", &tag));
    *output = new Dataset(ctx, input, std::move(tag));
  }

 private:
  class Dataset : public DatasetBase {
   public:
    explicit Dataset(OpKernelContext* ctx, const DatasetBase* input, string tag)
        : DatasetBase(DatasetContext(ctx)),
          input_(input),
          tag_(std::move(tag)) {
      input_->Ref();
    }

    ~Dataset() override { input_->Unref(); }

    std::unique_ptr<IteratorBase> MakeIteratorInternal(
        const string& prefix) const override {
      return std::unique_ptr<IteratorBase>(
          new Iterator({this, strings::StrCat(prefix, "::LatencyStats")}));
    }

    const DataTypeVector& output_dtypes() const override {
      return input_->output_dtypes();
    }
    const std::vector<PartialTensorShape>& output_shapes() const override {
      return input_->output_shapes();
    }

    string DebugString() const override {
      return "LatencyStatsDatasetOp::Dataset";
    }

    int64 Cardinality() const override { return input_->Cardinality(); }

   protected:
    Status AsGraphDefInternal(SerializationContext* ctx,
                              DatasetGraphDefBuilder* b,
                              Node** output) const override {
      Node* input_node;
      TF_RETURN_IF_ERROR(b->AddInputDataset(ctx, input_, &input_node));
      Node* tag_node;
      TF_RETURN_IF_ERROR(b->AddScalar(tag_, &tag_node));
      TF_RETURN_IF_ERROR(b->AddDataset(this, {input_node, tag_node}, output));
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
        tf_shared_lock l(mu_);
        uint64 start = ctx->env()->NowMicros();
        Status s = input_impl_->GetNext(ctx, out_tensors, end_of_sequence);
        uint64 end = ctx->env()->NowMicros();
        auto stats_aggregator = ctx->stats_aggregator();
        if (stats_aggregator && !*end_of_sequence) {
          ctx->stats_aggregator()->AddToHistogram(
              dataset()->tag_, {static_cast<double>(end - start)});
        }
        return s;
      }

     protected:
      std::shared_ptr<model::Node> CreateNode(
          IteratorContext* ctx, model::Node::Args args) const override {
        return model::MakeKnownRatioNode(std::move(args),
                                         /*ratio=*/1);
      }

      Status SaveInternal(IteratorStateWriter* writer) override {
        mutex_lock l(mu_);
        TF_RETURN_IF_ERROR(SaveInput(writer, input_impl_));
        return Status::OK();
      }

      Status RestoreInternal(IteratorContext* ctx,
                             IteratorStateReader* reader) override {
        mutex_lock l(mu_);
        TF_RETURN_IF_ERROR(RestoreInput(ctx, reader, input_impl_));
        return Status::OK();
      }

     private:
      mutex mu_;
      std::unique_ptr<IteratorBase> input_impl_ GUARDED_BY(mu_);
    };

    const DatasetBase* const input_;
    const string tag_;
  };
};

class BytesProducedStatsDatasetOp : public UnaryDatasetOpKernel {
 public:
  explicit BytesProducedStatsDatasetOp(OpKernelConstruction* ctx)
      : UnaryDatasetOpKernel(ctx) {}

  void MakeDataset(OpKernelContext* ctx, DatasetBase* input,
                   DatasetBase** output) override {
    string tag;
    OP_REQUIRES_OK(ctx, ParseScalarArgument(ctx, "tag", &tag));
    *output = new Dataset(ctx, input, std::move(tag));
  }

 private:
  class Dataset : public DatasetBase {
   public:
    explicit Dataset(OpKernelContext* ctx, const DatasetBase* input, string tag)
        : DatasetBase(DatasetContext(ctx)),
          input_(input),
          tag_(std::move(tag)) {
      input_->Ref();
    }

    ~Dataset() override { input_->Unref(); }

    std::unique_ptr<IteratorBase> MakeIteratorInternal(
        const string& prefix) const override {
      return std::unique_ptr<IteratorBase>(new Iterator(
          {this, strings::StrCat(prefix, "::BytesProducedStats")}));
    }

    const DataTypeVector& output_dtypes() const override {
      return input_->output_dtypes();
    }
    const std::vector<PartialTensorShape>& output_shapes() const override {
      return input_->output_shapes();
    }

    string DebugString() const override {
      return "BytesProducedStatsDatasetOp::Dataset";
    }

    int64 Cardinality() const override { return input_->Cardinality(); }

   protected:
    Status AsGraphDefInternal(SerializationContext* ctx,
                              DatasetGraphDefBuilder* b,
                              Node** output) const override {
      Node* input_node;
      TF_RETURN_IF_ERROR(b->AddInputDataset(ctx, input_, &input_node));
      Node* tag_node;
      TF_RETURN_IF_ERROR(b->AddScalar(tag_, &tag_node));
      TF_RETURN_IF_ERROR(b->AddDataset(this, {input_node, tag_node}, output));
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
        tf_shared_lock l(mu_);
        Status s = input_impl_->GetNext(ctx, out_tensors, end_of_sequence);
        auto stats_aggregator = ctx->stats_aggregator();
        if (stats_aggregator && s.ok() && !*end_of_sequence) {
          size_t total_bytes = 0;
          for (const Tensor& t : *out_tensors) {
            total_bytes += t.TotalBytes();
          }
          ctx->stats_aggregator()->AddToHistogram(
              dataset()->tag_, {static_cast<double>(total_bytes)});
        }
        return s;
      }

     protected:
      std::shared_ptr<model::Node> CreateNode(
          IteratorContext* ctx, model::Node::Args args) const override {
        return model::MakeKnownRatioNode(std::move(args),
                                         /*ratio=*/1);
      }

      Status SaveInternal(IteratorStateWriter* writer) override {
        mutex_lock l(mu_);
        TF_RETURN_IF_ERROR(SaveInput(writer, input_impl_));
        return Status::OK();
      }

      Status RestoreInternal(IteratorContext* ctx,
                             IteratorStateReader* reader) override {
        mutex_lock l(mu_);
        TF_RETURN_IF_ERROR(RestoreInput(ctx, reader, input_impl_));
        return Status::OK();
      }

     private:
      mutex mu_;
      std::unique_ptr<IteratorBase> input_impl_ GUARDED_BY(mu_);
    };

    const DatasetBase* const input_;
    const string tag_;
  };
};

REGISTER_KERNEL_BUILDER(
    Name("ExperimentalLatencyStatsDataset").Device(DEVICE_CPU),
    LatencyStatsDatasetOp);
REGISTER_KERNEL_BUILDER(
    Name("ExperimentalBytesProducedStatsDataset").Device(DEVICE_CPU),
    BytesProducedStatsDatasetOp);

}  // namespace
}  // namespace data
}  // namespace tensorflow
