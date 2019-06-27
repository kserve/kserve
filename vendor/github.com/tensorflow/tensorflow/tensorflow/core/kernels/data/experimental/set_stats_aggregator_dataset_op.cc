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
#include <memory>
#include "tensorflow/core/framework/dataset.h"
#include "tensorflow/core/framework/partial_tensor_shape.h"
#include "tensorflow/core/framework/stats_aggregator.h"
#include "tensorflow/core/framework/tensor.h"
#include "tensorflow/core/graph/graph_def_builder.h"
#include "tensorflow/core/lib/random/random.h"

namespace tensorflow {
namespace data {
namespace {

class StatsAggregatorWithTagAndPrefix : public StatsAggregator {
 public:
  StatsAggregatorWithTagAndPrefix(
      std::shared_ptr<StatsAggregator> stats_aggregator, const string& tag,
      const string& prefix)
      : wrapped_(stats_aggregator), tag_(tag), prefix_(prefix) {}

  void AddToHistogram(const string& name,
                      gtl::ArraySlice<double> values) override {
    if (!tag_.empty()) {
      wrapped_->AddToHistogram(strings::StrCat(tag_, "_", name), values);
    } else {
      wrapped_->AddToHistogram(name, values);
    }
  }

  void AddScalar(const string& name, float value) override {
    if (!tag_.empty()) {
      wrapped_->AddScalar(strings::StrCat(tag_, "_", name), value);
    } else {
      wrapped_->AddScalar(name, value);
    }
  }

  void EncodeToProto(Summary* out_summary) override {
    wrapped_->EncodeToProto(out_summary);
  }

  void IncrementCounter(const string& name, const string& label,
                        int64 val) override {
    if (!prefix_.empty()) {
      wrapped_->IncrementCounter(strings::StrCat(prefix_, "/", name), label,
                                 val);
    } else {
      wrapped_->IncrementCounter(strings::StrCat("/tensorflow/", name), label,
                                 val);
    }
  }

 private:
  std::shared_ptr<StatsAggregator> wrapped_;
  string tag_;
  string prefix_;
  TF_DISALLOW_COPY_AND_ASSIGN(StatsAggregatorWithTagAndPrefix);
};

class SetStatsAggregatorDatasetOp : public UnaryDatasetOpKernel {
 public:
  explicit SetStatsAggregatorDatasetOp(OpKernelConstruction* ctx)
      : UnaryDatasetOpKernel(ctx) {}

  void MakeDataset(OpKernelContext* ctx, DatasetBase* input,
                   DatasetBase** output) override {
    StatsAggregatorResource* stats_aggregator_resource;
    OP_REQUIRES_OK(ctx, LookupResource(ctx, HandleFromInput(ctx, 1),
                                       &stats_aggregator_resource));
    core::ScopedUnref unref_stats_aggregator(stats_aggregator_resource);
    string tag;
    OP_REQUIRES_OK(ctx, ParseScalarArgument(ctx, "tag", &tag));
    string prefix;
    OP_REQUIRES_OK(ctx, ParseScalarArgument(ctx, "counter_prefix", &prefix));

    *output = new Dataset(ctx, input, ctx->input(1), stats_aggregator_resource,
                          tag, prefix);
  }

 private:
  class Dataset : public DatasetBase {
   public:
    explicit Dataset(OpKernelContext* ctx, const DatasetBase* input,
                     const Tensor& resource_handle,
                     StatsAggregatorResource* stats_aggregator_resource,
                     const string& tag, const string& prefix)
        : DatasetBase(DatasetContext(ctx)),
          input_(input),
          resource_handle_(resource_handle),
          stats_aggregator_resource_(stats_aggregator_resource),
          tag_(tag),
          prefix_(prefix) {
      input_->Ref();
      stats_aggregator_resource_->Ref();
    }

    ~Dataset() override {
      input_->Unref();
      stats_aggregator_resource_->Unref();
    }

    std::unique_ptr<IteratorBase> MakeIteratorInternal(
        const string& prefix) const override {
      return std::unique_ptr<IteratorBase>(new Iterator(
          {this, strings::StrCat(prefix, "::SetStatsAggregator")}));
    }

    const DataTypeVector& output_dtypes() const override {
      return input_->output_dtypes();
    }
    const std::vector<PartialTensorShape>& output_shapes() const override {
      return input_->output_shapes();
    }

    string DebugString() const override {
      return "SetStatsAggregatorDatasetOp::Dataset";
    }

    int64 Cardinality() const override { return input_->Cardinality(); }

   protected:
    Status AsGraphDefInternal(SerializationContext* ctx,
                              DatasetGraphDefBuilder* b,
                              Node** output) const override {
      Node* input_graph_node = nullptr;
      TF_RETURN_IF_ERROR(b->AddInputDataset(ctx, input_, &input_graph_node));
      Node* resource_handle_node = nullptr;
      TF_RETURN_IF_ERROR(b->AddTensor(resource_handle_, &resource_handle_node));
      Node* tag_node = nullptr;
      TF_RETURN_IF_ERROR(b->AddScalar(tag_, &tag_node));
      Node* prefix_node = nullptr;
      TF_RETURN_IF_ERROR(b->AddScalar(prefix_, &prefix_node));
      TF_RETURN_IF_ERROR(b->AddDataset(
          this, {input_graph_node, resource_handle_node, tag_node, prefix_node},
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
        mutex_lock l(mu_);
        StatsAggregatorResource* stats_aggregator_resource =
            dataset()->stats_aggregator_resource_;
        IteratorContext::Params params(ctx);
        params.stats_aggregator = std::shared_ptr<StatsAggregator>(
            new StatsAggregatorWithTagAndPrefix(
                stats_aggregator_resource->stats_aggregator(), dataset()->tag_,
                dataset()->prefix_));
        IteratorContext iter_ctx(std::move(params));
        return input_impl_->GetNext(&iter_ctx, out_tensors, end_of_sequence);
      }

     protected:
      std::shared_ptr<model::Node> CreateNode(
          IteratorContext* ctx, model::Node::Args args) const override {
        return model::MakeKnownRatioNode(std::move(args),
                                         /*ratio=*/1);
      }

      Status SaveInternal(IteratorStateWriter* writer) override {
        return errors::Unimplemented(dataset()->DebugString(),
                                     " does not support checkpointing");
      }

      Status RestoreInternal(IteratorContext* ctx,
                             IteratorStateReader* reader) override {
        return errors::Unimplemented(dataset()->DebugString(),
                                     " does not support checkpointing");
      }

     private:
      mutex mu_;
      std::unique_ptr<IteratorBase> input_impl_ GUARDED_BY(mu_);
    };

    const DatasetBase* const input_;
    const Tensor resource_handle_;
    StatsAggregatorResource* stats_aggregator_resource_;
    string tag_;
    string prefix_;
  };
};

REGISTER_KERNEL_BUILDER(
    Name("ExperimentalSetStatsAggregatorDataset").Device(DEVICE_CPU),
    SetStatsAggregatorDatasetOp);
}  // namespace
}  // namespace data
}  // namespace tensorflow
