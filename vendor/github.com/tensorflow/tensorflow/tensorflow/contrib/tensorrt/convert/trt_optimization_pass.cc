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

#include "tensorflow/contrib/tensorrt/convert/trt_optimization_pass.h"
#include "tensorflow/contrib/tensorrt/convert/convert_graph.h"
#include "tensorflow/contrib/tensorrt/convert/utils.h"
#include "tensorflow/core/grappler/clusters/cluster.h"
#include "tensorflow/core/grappler/grappler_item.h"
#include "tensorflow/core/grappler/optimizers/custom_graph_optimizer_registry.h"
#include "tensorflow/core/lib/strings/numbers.h"
#include "tensorflow/core/lib/strings/str_util.h"
#include "tensorflow/core/lib/strings/strcat.h"
#include "tensorflow/core/platform/logging.h"
#include "tensorflow/core/platform/stacktrace.h"

#if GOOGLE_CUDA
#if GOOGLE_TENSORRT
namespace tensorflow {
namespace tensorrt {
namespace convert {
// TODO(sami): Remove VLOG messages once the code matures
using tensorflow::str_util::Uppercase;
using tensorflow::strings::StrAppend;
using tensorflow::strings::StrCat;

tensorflow::Status TRTOptimizationPass::Init(
    const tensorflow::RewriterConfig_CustomGraphOptimizer* config) {
  VLOG(1) << "Called INIT for " << name_ << " with config = " << config;
  if (config == nullptr) {
    return tensorflow::Status::OK();
  }
  const auto params = config->parameter_map();
  if (params.count("minimum_segment_size")) {
    minimum_segment_size_ = params.at("minimum_segment_size").i();
  }
  if (params.count("max_batch_size")) {
    maximum_batch_size_ = params.at("max_batch_size").i();
  }
  if (params.count("is_dynamic_op")) {
    is_dynamic_op_ = params.at("is_dynamic_op").b();
  }
  if (params.count("cached_engine_batches")) {
    auto batch_vec = params.at("cached_engine_batches").list();
    batches_.reserve(batch_vec.i_size());
    for (const auto i : batch_vec.i()) {
      batches_.push_back(i);
    }
  }
  if (params.count("maximum_cached_engines")) {
    max_cached_batches_ = params.at("maximum_cached_engines").i();
  }
  if (params.count("max_workspace_size_bytes")) {
    max_workspace_size_bytes_ = params.at("max_workspace_size_bytes").i();
  }
  if (params.count("precision_mode")) {
    TF_RETURN_IF_ERROR(GetPrecisionMode(
        Uppercase(params.at("precision_mode").s()), &precision_mode_));
  }
  if (params.count("use_calibration")) {
    use_calibration_ = params.at("use_calibration").b();
  }
  return tensorflow::Status::OK();
}

void TRTOptimizationPass::PrintDebugInfo(
    tensorflow::grappler::Cluster* cluster,
    const tensorflow::grappler::GrapplerItem& item) {
  LOG(INFO) << "Cluster = " << cluster;
  string offset("  ");
  string offset2 = StrCat(offset, offset);
  string offset3 = StrCat(offset2, offset);
  string offset4 = StrCat(offset2, offset2);
  if (cluster) {
    LOG(INFO) << offset << "type             = " << cluster->type();
    LOG(INFO) << offset << "num warmup steps = " << cluster->NumWarmupSteps();
    const auto dev_names = cluster->GetDeviceNames();
    if (dev_names.size()) {
      LOG(INFO) << offset << " Device names:";
      for (const auto s : dev_names) {
        LOG(INFO) << offset2 << s;
      }
    }
    std::unordered_map<string, uint64> peak_mem;
    auto status = cluster->GetPeakMemoryUsage(&peak_mem);
    if (status == tensorflow::Status::OK()) {
      LOG(INFO) << offset << "Peak Memory Usage :";
      for (auto s : peak_mem) {
        LOG(INFO) << offset2 << s.first << " = " << s.second;
      }
    }

    const auto dev_props = cluster->GetDevices();
    if (dev_props.size()) {
      LOG(INFO) << offset << "Device properties:";
      for (auto k : dev_props) {
        LOG(INFO) << offset2 << k.first;
        const auto& dt = k.second;
        LOG(INFO) << offset3 << "type          = " << dt.type();
        LOG(INFO) << offset3 << "vendor        = " << dt.vendor();
        LOG(INFO) << offset3 << "model         = " << dt.model();
        LOG(INFO) << offset3 << "frequency     = " << dt.frequency();
        LOG(INFO) << offset3 << "num cores     = " << dt.num_cores();
        LOG(INFO) << offset3 << "num registers = " << dt.num_registers();
        LOG(INFO) << offset3 << "L1 cache size = " << dt.l1_cache_size();
        LOG(INFO) << offset3 << "L2 cache size = " << dt.l2_cache_size();
        LOG(INFO) << offset3 << "L3 cache size = " << dt.l3_cache_size();
        LOG(INFO) << offset3 << "SHMem per SMP = "
                  << dt.shared_memory_size_per_multiprocessor();
        LOG(INFO) << offset3 << "memory size   = " << dt.memory_size();
        LOG(INFO) << offset3 << "bandwidth     = " << dt.bandwidth();
        if (dt.environment_size()) {
          LOG(INFO) << offset3 << "environment   :";
          for (const auto e : dt.environment()) {
            LOG(INFO) << offset4 << e.first << " = " << e.second;
          }
        }
      }
    }
  }
  LOG(INFO) << "item: " << item.id;
  if (item.feed.size()) {
    LOG(INFO) << offset << "Feeds  :";
    for (const auto& f : item.feed) {
      const auto& shape = f.second.shape();
      LOG(INFO) << offset2 << f.first << " = shaped " << shape.DebugString();
    }
  } else {
    LOG(INFO) << offset << "No Feeds";
  }
  if (item.fetch.size()) {
    LOG(INFO) << offset << "Fetches  :";
    for (const auto& f : item.fetch) {
      LOG(INFO) << offset2 << f;
    }
  } else {
    LOG(INFO) << offset << "No Fetches";
  }

  if (item.init_ops.size()) {
    LOG(INFO) << offset << "init ops  :";
    for (const auto& f : item.init_ops) {
      LOG(INFO) << offset2 << f;
    }
  } else {
    LOG(INFO) << offset << "No init ops";
  }
  LOG(INFO) << "Save Op = " << item.save_op;
  LOG(INFO) << "Restore Op = " << item.restore_op;
  LOG(INFO) << "save_restore_loc_tensor = " << item.save_restore_loc_tensor;
  if (item.keep_ops.size()) {
    LOG(INFO) << offset << "keep ops  :";
    for (const auto& f : item.keep_ops) {
      LOG(INFO) << offset2 << f;
    }
  } else {
    LOG(INFO) << offset << "No keep ops";
  }
  for (const auto dev : cluster->GetDeviceSet()->devices()) {
    const auto& pname = dev->parsed_name();
    LOG(INFO) << "Device name= " << dev->name()
              << " parsedname job= " << pname.job << " id= " << pname.id
              << " has_id: " << pname.has_id << " has_job: " << pname.has_job
              << "has_type: " << pname.has_type << " type =" << pname.type;
  }
}

tensorflow::Status TRTOptimizationPass::Optimize(
    tensorflow::grappler::Cluster* cluster,
    const tensorflow::grappler::GrapplerItem& item, GraphDef* optimized_graph) {
  VLOG(1) << "Called TRTOptimization Pass " << name_;
  // This is a hack to workaround optimizer issue. MetaOptimizer calls
  // optimization passes on function objects as well, we should not modify
  // generated funcdefs! This is fragile but we don't have any other option
  // until framework fixes it.
  if (item.id != "tf_graph") {
    LOG(WARNING) << name_
                 << " is probably called on funcdef! This optimizer must *NOT* "
                    "be called on function objects.";
    *optimized_graph = item.graph;
    return tensorflow::Status::OK();
  }
  if (VLOG_IS_ON(3)) {
    LOG(INFO) << CurrentStackTrace();
    PrintDebugInfo(cluster, item);
  }
  int max_dim = -1;
  if (item.feed.size()) {
    for (const auto& f : item.feed) {
      const auto& shape = f.second.shape();
      if (shape.dims() > 0) {
        if (shape.dim_size(0) > max_dim) max_dim = shape.dim_size(0);
      }
    }
  }
  if (maximum_batch_size_ < 0) {  // automatic batch size from input
    if (max_dim > 0) {
      maximum_batch_size_ = max_dim;
      VLOG(1) << "Setting maximum batch size to " << max_dim;
    } else {
      maximum_batch_size_ = 128;
      LOG(WARNING) << "Maximum batch size is not set"
                      " and can't be deduced from inputs setting it to"
                   << maximum_batch_size_
                   << ". Suggest configuring it from configuration parameters";
    }
  } else {
    if (max_dim > maximum_batch_size_) {
      LOG(WARNING) << "Configured batch size " << maximum_batch_size_
                   << " is less than input batch size " << max_dim
                   << " adjusting maximum batch size to match input batch size";
    }
  }
  tensorflow::grappler::GraphProperties static_graph_properties(item);
  TF_RETURN_IF_ERROR(static_graph_properties.InferStatically(true));
  tensorflow::tensorrt::convert::ConversionParams cp;

  if (use_calibration_ && precision_mode_ != INT8MODE) {
    VLOG(1) << "Calibration with FP32 or FP16 is not implemented. "
            << "Falling back to use_calibration = False."
            << "Note that the default value of use_calibration is True.";
    use_calibration_ = false;
  }

  std::vector<string> nodes_to_preserve;
  for (const auto& n : item.NodesToPreserve()) {
    auto tokens = str_util::Split(n, ":");
    string s = tokens.at(0);
    for (int i = 1; i < tokens.size() - 1; ++i) {
      StrAppend(&s, ":", tokens.at(i));
    }
    int dumm_port = -1;
    // If the last token is not an integer, it must be part of the name.
    // Otherwise it is port number.
    if (tokens.size() > 1 &&
        !strings::safe_strto32(tokens.back(), &dumm_port)) {
      StrAppend(&s, ":", tokens.back());
    }
    nodes_to_preserve.push_back(s);
  }
  cp.input_graph_def = &item.graph;
  cp.output_names = &nodes_to_preserve;
  cp.max_batch_size = maximum_batch_size_;
  cp.max_workspace_size_bytes = max_workspace_size_bytes_;
  cp.output_graph_def = optimized_graph;
  cp.precision_mode = precision_mode_;
  cp.minimum_segment_size = minimum_segment_size_;
  cp.graph_properties = &static_graph_properties;
  cp.cluster = cluster;
  cp.is_dyn_op = is_dynamic_op_;
  cp.cached_engine_batches = batches_;
  cp.max_cached_engines = max_cached_batches_;
  cp.use_calibration = use_calibration_;
  auto status = tensorflow::tensorrt::convert::ConvertAfterShapes(cp);
  VLOG(1) << "Returning from " << name_;
  return status;
}

void TRTOptimizationPass::Feedback(
    tensorflow::grappler::Cluster* cluster,
    const tensorflow::grappler::GrapplerItem& item,
    const GraphDef& optimized_graph, double result) {}

}  // namespace convert
}  // namespace tensorrt
}  // namespace tensorflow

class VerboseCustomGraphOptimizerRegistrar
    : public tensorflow::grappler::CustomGraphOptimizerRegistrar {
 public:
  VerboseCustomGraphOptimizerRegistrar(
      const tensorflow::grappler::CustomGraphOptimizerRegistry::Creator& cr,
      const tensorflow::string& name)
      : tensorflow::grappler::CustomGraphOptimizerRegistrar(cr, name) {
    VLOG(1) << "Constructing a CustomOptimizationPass registration object for "
            << name;
  }
};

static VerboseCustomGraphOptimizerRegistrar TRTOptimizationPass_Registrar(
    []() {
      VLOG(1)
          << "Instantiating CustomOptimizationPass object TensorRTOptimizer";
      return new tensorflow::tensorrt::convert::TRTOptimizationPass(
          "TensorRTOptimizer");
    },
    ("TensorRTOptimizer"));

#endif
#endif
