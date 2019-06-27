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
#include "tensorflow/core/common_runtime/function.h"
#include "tensorflow/core/common_runtime/optimization_registry.h"
#include "tensorflow/core/common_runtime/placer.h"
#include "tensorflow/core/common_runtime/rendezvous_mgr.h"
#include "tensorflow/core/framework/function.h"
#include "tensorflow/core/framework/graph_to_functiondef.h"
#include "tensorflow/core/framework/op_kernel.h"
#include "tensorflow/core/framework/tensor.h"
#include "tensorflow/core/framework/types.h"
#include "tensorflow/core/graph/graph.h"
#include "tensorflow/core/graph/graph_constructor.h"
#include "tensorflow/core/graph/graph_partition.h"
#include "tensorflow/core/grappler/clusters/virtual_cluster.h"
#include "tensorflow/core/grappler/grappler_item.h"
#include "tensorflow/core/grappler/optimizers/meta_optimizer.h"
#include "tensorflow/core/grappler/utils/functions.h"
#include "tensorflow/core/protobuf/config.pb.h"
#include "tensorflow/core/protobuf/rewriter_config.pb.h"
#include "tensorflow/core/util/ptr_util.h"
#include "tensorflow/core/util/reffed_status_callback.h"

#if GOOGLE_CUDA
#include "tensorflow/stream_executor/stream.h"
#endif  // GOOGLE_CUDA

namespace tensorflow {
typedef FunctionLibraryRuntime::Handle FHandle;

namespace {
// A `PartitionedCallOp` asynchronously executes a function, potentially across
// multiple devices but within a single process. The kernel places and
// partitions a given function's underlying graph, and executes each of the
// partitioned subgraphs as a function.
//
// TODO(akshayka): Support distributed execution.
class PartitionedCallOp : public AsyncOpKernel {
 public:
  explicit PartitionedCallOp(OpKernelConstruction* ctx) : AsyncOpKernel(ctx) {
    OP_REQUIRES_OK(ctx, ctx->GetAttr("f", &func_));
    string deprecated_config_serialized;
    OP_REQUIRES_OK(ctx, ctx->GetAttr("config", &deprecated_config_serialized));
    string config_proto_serialized;
    OP_REQUIRES_OK(ctx, ctx->GetAttr("config_proto", &config_proto_serialized));
    OP_REQUIRES(
        ctx,
        deprecated_config_serialized.empty() || config_proto_serialized.empty(),
        errors::InvalidArgument("Provided both 'config' and 'config_proto' but "
                                "only one should be provided.  Note the "
                                "'config' option is deprecated."));
    if (!deprecated_config_serialized.empty()) {
      OP_REQUIRES(ctx,
                  config_proto_.mutable_graph_options()
                      ->mutable_rewrite_options()
                      ->ParseFromString(deprecated_config_serialized),
                  errors::InvalidArgument("Unable to parse config string as "
                                          "tensorflow::RewriteOptions proto."));
    } else {
      OP_REQUIRES(
          ctx, config_proto_.ParseFromString(config_proto_serialized),
          errors::InvalidArgument("Unable to parse config_proto string as "
                                  "tensorflow::ConfigProto proto."));
    }
    OP_REQUIRES_OK(ctx, ctx->GetAttr("executor_type", &executor_type_));
  }

  ~PartitionedCallOp() override {}

  void ComputeAsync(OpKernelContext* ctx, DoneCallback done) override {
    FunctionLibraryRuntime* lib = ctx->function_library();
    OP_REQUIRES_ASYNC(ctx, lib != nullptr,
                      errors::Internal("No function library is provided."),
                      done);

    OpInputList args;
    OP_REQUIRES_OK_ASYNC(ctx, ctx->input_list("args", &args), done);

    // The function body's graph is placed and partitioned the first time
    // `ComputeAsync` is invoked; every subsequent invocation calls each
    // of the function shards yielded by partitioning.
    //
    // The partitioning step yields a set of devices on which to run the
    // function, and exactly one function shard is created for each device
    // Inputs and outputs are pinned to the local device, for simplicity.
    //
    // TODO(akshayka): Support re-sharding the function on subsequent calls,
    // via, e.g., virtual device annotations and a list of device names supplied
    // through an attribute.
    //
    // TODO(akshayka): Add a fastpath for functions that execute on a single
    // device.
    {
      mutex_lock l(mu_);
      if (function_handles_.find(lib) == function_handles_.end()) {
        // TODO(b/37549631): Because this kernel may correspond to a stateful
        // op, it may be shared by multiple subgraphs, which in turn may have
        // different `FunctionLibraryRuntime` objects and therefore different
        // `FHandle` namespaces. As such, we partition on a per-FLR basis.
        FunctionLibraryRuntime::InstantiateOptions opts;
        FHandle handle;
        OP_REQUIRES_OK_ASYNC(
            ctx,
            lib->Instantiate(func_.name(), AttrSlice(&func_.attr()), opts,
                             &handle),
            done);
        const FunctionBody* fbody = lib->GetFunctionBody(handle);
        OP_REQUIRES_ASYNC(ctx, fbody != nullptr,
                          errors::Internal("Could not find handle ", handle),
                          done);
        OP_REQUIRES_ASYNC(
            ctx, args.size() == fbody->arg_nodes.size(),
            errors::InvalidArgument(
                "Wrong number of arguments to the op; function expects ",
                fbody->arg_nodes.size(), " but PartitionedCall received ",
                args.size()),
            done);
        // We need to pass global op_registry as default_registry when creating
        // graph. So that graph optimization passes can lookup all possible ops
        // by name.
        auto graph = tensorflow::MakeUnique<Graph>(fbody->graph->flib_def());
        FunctionLibraryDefinition global_flib(OpRegistry::Global(), {});
        TF_CHECK_OK(graph->AddFunctionLibrary(global_flib.ToProto()));
        CopyGraph(*fbody->graph, graph.get());
        OP_REQUIRES_OK_ASYNC(ctx, PinResourceArgs(graph.get(), args), done);

        DeviceSet device_set;
        for (auto d : lib->device_mgr()->ListDevices()) {
          device_set.AddDevice(d);
        }

        // The FunctionLibraryRuntime's library cannot be mutated from within
        // an OpKernel, so functions are instantiated in an overlay library.
        OP_REQUIRES_ASYNC(
            ctx, overlay_libs_.find(lib) == overlay_libs_.end(),
            errors::Internal("Found an overlay library but did not "
                             "find cached function partitions; "
                             "this indicates a bug."),
            done);
        // We do not need a full function library in the overlay, we just keep a
        // subset that is reachable from the instantiated function.
        FunctionLibraryDefinition* overlay_lib = new FunctionLibraryDefinition(
            grappler::ReachableFunctionLibraryDefinition(
                *lib->GetFunctionLibraryDefinition(), fbody->fdef));
        overlay_libs_.emplace(lib, overlay_lib);

        GraphOptimizationPassOptions optimization_options;
        // TODO(akshayka): Thread SessionOptions (if any) into this kernel, or
        // make it possible to specify the relevant options via attributes.
        SessionOptions session_options;
        session_options.env = ctx->env();
        optimization_options.session_options = &session_options;
        optimization_options.graph = &graph;
        optimization_options.flib_def = overlay_lib;
        optimization_options.device_set = &device_set;
        OP_REQUIRES_OK_ASYNC(
            ctx,
            OptimizationPassRegistry::Global()->RunGrouping(
                OptimizationPassRegistry::PRE_PLACEMENT, optimization_options),
            done);

        // Make the FunctionLibraryRuntime's device the default device if
        // nothing else is hard coded. This allows the same function definition
        // to be specialized to different devices depending on the
        // PartitionedCallOp's device.
        Placer placer(graph.get(), &device_set,
                      nullptr, /* No session options */
                      lib->device() /* Default device */);
        OP_REQUIRES_OK_ASYNC(ctx, placer.Run(), done);
        OP_REQUIRES_OK_ASYNC(
            ctx,
            OptimizationPassRegistry::Global()->RunGrouping(
                OptimizationPassRegistry::POST_PLACEMENT, optimization_options),
            done);

        Device* cpu_device;
        OP_REQUIRES_OK_ASYNC(
            ctx, lib->device_mgr()->LookupDevice("CPU:0", &cpu_device), done);

        // Run grappler passes on the graph. It is possible that these are
        // optimized by the graph executor already.
        Status optimized = OptimizeGraph(ctx, fbody->ret_nodes, overlay_lib,
                                         device_set, cpu_device, &graph);
        if (!optimized.ok()) {
          LOG(WARNING) << "Grappler optimization failed. Error: "
                       << optimized.error_message();
        }

        OP_REQUIRES_OK_ASYNC(
            ctx,
            OptimizationPassRegistry::Global()->RunGrouping(
                OptimizationPassRegistry::POST_REWRITE_FOR_EXEC,
                optimization_options),
            done);

        std::unordered_map<string, std::unique_ptr<Graph>> subgraphs;
        OP_REQUIRES_OK_ASYNC(
            ctx, PartitionHelper(device_set, std::move(graph), &subgraphs),
            done);
        if (ctx->graph_collector() != nullptr) {
          for (const auto& pair : subgraphs) {
            GraphDef def;
            pair.second->ToGraphDef(&def);
            ctx->graph_collector()->CollectGraph(def);
          }
        }
        optimization_options.graph = nullptr;
        optimization_options.device_set = nullptr;
        optimization_options.partition_graphs = &subgraphs;
        OP_REQUIRES_OK_ASYNC(ctx,
                             OptimizationPassRegistry::Global()->RunGrouping(
                                 OptimizationPassRegistry::POST_PARTITIONING,
                                 optimization_options),
                             done);

        auto handles = tensorflow::MakeUnique<gtl::FlatMap<string, FHandle>>();
        for (const auto& pair : subgraphs) {
          // TODO(akshayka): Fail gracefully if the set of devices corresponds
          // to more than one address space.
          const string& target = pair.first;
          const auto& subgraph = pair.second;
          OP_REQUIRES_OK_ASYNC(
              ctx, UpdateArgAndRetMetadata(target, subgraph.get()), done);
          FunctionDef shard;
          string unique_name = UniquifyFunctionName(overlay_lib, func_.name());
          OP_REQUIRES_OK_ASYNC(
              ctx, GraphToFunctionDef(*subgraph, unique_name, &shard), done);
          OP_REQUIRES_OK_ASYNC(ctx, overlay_lib->AddFunctionDef(shard), done);
          FunctionLibraryRuntime::InstantiateOptions opts;
          opts.executor_type = executor_type_;
          opts.target = target;
          opts.overlay_lib = overlay_lib;
          FHandle handle;
          OP_REQUIRES_OK_ASYNC(
              ctx,
              lib->Instantiate(unique_name, AttrSlice(&shard.attr()), opts,
                               &handle),
              done);
          handles->emplace(target, handle);
        }

        function_handles_.emplace(lib, std::move(handles));
      }
    }
    ExecuteFunctions(lib, ctx, args, std::move(done));
  }

 private:
  typedef std::pair<string, FHandle> DeviceAndFHandle;
  typedef std::pair<std::vector<int>, std::vector<int>> ArgAndRetIndices;
  typedef std::pair<std::vector<AllocatorAttributes>,
                    std::vector<AllocatorAttributes>>
      ArgAndRetAllocAttrs;

  // Pins each arg that emits a `DT_RESOURCE` tensor to the device on which the
  // corresponding resource lives. This ensures that the Placer assigns ops that
  // access these resources to the appropriate devices.
  Status PinResourceArgs(Graph* graph, const OpInputList& args) {
    for (Node* node : graph->op_nodes()) {
      string node_type = node->type_string();
      if (node_type == FunctionLibraryDefinition::kArgOp) {
        const AttrValue* attr_value;
        TF_RETURN_IF_ERROR(node->attrs().Find("index", &attr_value));
        int index = attr_value->i();
        TF_RETURN_IF_ERROR(node->attrs().Find("T", &attr_value));
        DataType dtype = attr_value->type();
        if (dtype != args[index].dtype()) {
          return errors::InvalidArgument("For argument ", index, " expected ",
                                         DataTypeString(dtype), " tensor, got ",
                                         DataTypeString(args[index].dtype()),
                                         " instead.");
        }
        if (dtype == DT_RESOURCE) {
          const ResourceHandle& handle = args[index].flat<ResourceHandle>()(0);
          node->set_assigned_device_name(handle.device());
        }
      }
    }
    return Status::OK();
  }

  // Partitions `graph` and populates `subgraphs` with the partitions.
  Status PartitionHelper(
      const DeviceSet& device_set, std::unique_ptr<Graph> graph,
      std::unordered_map<string, std::unique_ptr<Graph>>* subgraphs) {
    PartitionOptions partition_options;
    partition_options.node_to_loc = [](const Node* node) {
      // TODO(akshayka): To better support the distributed case, first split
      // the graph by worker (e.g,. using the master session's
      // `SplitByWorker` policy), and then recursively partition the
      // per-worker shards at the remote worker(s).
      return node->assigned_device_name();
    };
    int64 edge_name_counter = 0;
    partition_options.new_name = [&edge_name_counter](const string& prefix) {
      return strings::StrCat(prefix, "/_", ++edge_name_counter);
    };
    partition_options.get_incarnation =
        [&device_set](const string& name) -> int64 {
      const Device* d = device_set.FindDeviceByName(name);
      if (d == nullptr) {
        return PartitionOptions::kIllegalIncarnation;
      } else {
        return d->attributes().incarnation();
      }
    };
    partition_options.control_flow_added = false;
    std::unordered_map<string, GraphDef> partitions;
    TF_RETURN_IF_ERROR(Partition(partition_options, graph.get(), &partitions));

    VLOG(3) << "Partitioned function '" << func_.name() << "', yielding "
            << partitions.size() << " shards.";

    for (const auto& partition : partitions) {
      std::unique_ptr<Graph> subgraph(new Graph(graph->flib_def()));
      FunctionLibraryDefinition global_flib(OpRegistry::Global(), {});
      TF_CHECK_OK(subgraph->AddFunctionLibrary(global_flib.ToProto()));
      GraphConstructorOptions opts;
      opts.allow_internal_ops = true;
      opts.expect_device_spec = true;
      const string& device = partition.first;
      const GraphDef& graph_def = partition.second;
      TF_RETURN_IF_ERROR(
          ConvertGraphDefToGraph(opts, graph_def, subgraph.get()));
      subgraphs->emplace(device, std::move(subgraph));
    }

    return Status::OK();
  }

  // Each subgraph produced by partitioning the function body contains a subset
  // of the original `Arg` and `Retval` nodes. This function performs
  // bookkeeping to track which `Arg` and `Retval` nodes were placed on a
  // particular device / subgraph.
  //
  // More specifically, this function
  //  (1) rewrites the indices of the `Arg` and `Retval` nodes placed on a
  //      particular device,
  //  (2) records the subsets of `Arg` and `Retval` nodes assigned to the
  //      device, and
  //  (3) records which `Arg` and `Retval` nodes live in host memory.
  Status UpdateArgAndRetMetadata(const string& device, Graph* subgraph) {
    ArgAndRetIndices indices;
    std::vector<int>* arg_indices = &indices.first;
    std::vector<int>* ret_indices = &indices.second;
    std::vector<std::pair<Node*, int>> arg_nodes;
    std::vector<std::pair<Node*, int>> ret_nodes;
    const AttrValue* attr_value;

    // Find the Arg and Retval nodes, along with their corresponding indices
    // in the original function.
    for (Node* node : subgraph->op_nodes()) {
      string node_type = node->type_string();
      if (node_type == FunctionLibraryDefinition::kArgOp) {
        TF_RETURN_IF_ERROR(node->attrs().Find("index", &attr_value));
        int index = attr_value->i();
        arg_indices->push_back(index);
        arg_nodes.push_back(std::make_pair(node, index));
      } else if (node_type == FunctionLibraryDefinition::kRetOp) {
        TF_RETURN_IF_ERROR(node->attrs().Find("index", &attr_value));
        int index = attr_value->i();
        ret_indices->push_back(index);
        ret_nodes.push_back(std::make_pair(node, index));
      }
    }

    for (int i = 0; i < arg_nodes.size(); ++i) {
      Node* arg = arg_nodes[i].first;
      arg->AddAttr("index", i);
      TF_RETURN_IF_ERROR(arg->attrs().Find("T", &attr_value));
      AllocatorAttributes alloc_attr;
      DataType type = attr_value->type();
      if (MTypeFromDType(type) == HOST_MEMORY) {
        alloc_attr.set_on_host(true);
      }
      arg_and_ret_alloc_attrs_[device].first.push_back(alloc_attr);
    }
    for (int i = 0; i < ret_nodes.size(); ++i) {
      Node* ret = ret_nodes[i].first;
      ret->AddAttr("index", i);
      TF_RETURN_IF_ERROR(ret->attrs().Find("T", &attr_value));
      AllocatorAttributes alloc_attr;
      DataType type = attr_value->type();
      if (MTypeFromDType(type) == HOST_MEMORY) {
        alloc_attr.set_on_host(true);
      }
      arg_and_ret_alloc_attrs_[device].second.push_back(alloc_attr);
    }

    // If this kernel execution corresponds to a StatefulPartitionedCallOp,
    // `arg_and_ret_indices_` might have been populated by a previous
    // invocation.
    if (arg_and_ret_indices_.find(device) == arg_and_ret_indices_.end()) {
      arg_and_ret_indices_.emplace(device, indices);
    }
    return Status::OK();
  }

  std::vector<Tensor> GetArgsForIndices(const std::vector<int>& indices,
                                        const OpInputList& arguments) {
    std::vector<Tensor> args;
    args.reserve(indices.size());
    for (int i : indices) {
      args.push_back(arguments[i]);
    }
    return args;
  }

  void ExecuteFunctions(FunctionLibraryRuntime* lib, OpKernelContext* ctx,
                        const OpInputList& op_args, DoneCallback done)
      LOCKS_EXCLUDED(mu_) {
    const gtl::FlatMap<string, FHandle>* handles;
    {
      mutex_lock l(mu_);
      handles = function_handles_[lib].get();
    }
    if (handles->empty()) {
      // Trivial case where the function body is empty.
      ctx->SetStatus(Status::OK());
      done();
      return;
    }

    const string& local_device_name = lib->device()->name();
    FunctionLibraryRuntime::Options opts;
    opts.step_id = ctx->step_id();
    opts.step_container = ctx->step_container();
    opts.cancellation_manager = ctx->cancellation_manager();
    opts.stats_collector = ctx->stats_collector();
    // TODO(akshayka): Consider selecting a runner on a per-device basis, i.e.,
    // using device-specific threadpools when available.
    opts.runner = ctx->runner();
    opts.source_device = local_device_name;
    opts.allow_dead_tensors = true;
    // TODO(akshayka): Accommodate the multiple-worker scenario by adding the
    // constructed rendezvous to a rendezvous manager.
    Rendezvous* rendez = new IntraProcessRendezvous(lib->device_mgr());
    opts.rendezvous = rendez;

    StatusCallback callback = std::bind(
        [](Rendezvous* rendez, DoneCallback& done, const Status& status) {
          rendez->Unref();
          done();
        },
        rendez, std::move(done), std::placeholders::_1);
    auto* refcounted_done = new ReffedStatusCallback(std::move(callback));
    for (int i = 0; i < handles->size(); ++i) {
      refcounted_done->Ref();
    }

    for (const auto& pair : *handles) {
      const string& target = pair.first;
      FHandle handle = pair.second;
      VLOG(3) << "Running function shard on device " << target;
      ArgAndRetIndices indices = arg_and_ret_indices_[target];
      ArgAndRetAllocAttrs alloc_attrs = arg_and_ret_alloc_attrs_[target];
      const std::vector<int>& arg_indices = indices.first;
      const std::vector<int>& ret_indices = indices.second;
      opts.args_alloc_attrs = alloc_attrs.first;
      opts.rets_alloc_attrs = alloc_attrs.second;
      if (target == local_device_name) {
        opts.remote_execution = false;
        std::vector<Tensor> args = GetArgsForIndices(arg_indices, op_args);
        std::vector<Tensor>* rets = new std::vector<Tensor>;
        lib->Run(
            opts, handle, args, rets,
            [rets, ret_indices, refcounted_done, ctx](const Status& status) {
              if (!status.ok()) {
                VLOG(3) << "Local execution failed: " << status;
                ctx->SetStatus(status);
              } else {
                for (int i = 0; i < rets->size(); ++i) {
                  ctx->set_output(ret_indices[i], (*rets)[i]);
                }
              }
              delete rets;
              VLOG(3) << "Finished local execution.";
              refcounted_done->Unref();
            });
      } else {
        opts.remote_execution = true;
        std::vector<Tensor> args = GetArgsForIndices(arg_indices, op_args);
        std::vector<Tensor>* rets = new std::vector<Tensor>;
        lib->Run(
            opts, handle, args, rets,
            [rets, ret_indices, refcounted_done, ctx](const Status& status) {
              if (!status.ok()) {
                VLOG(3) << "Remote execution failed: " << status;
                ctx->SetStatus(status);
              } else {
                for (int i = 0; i < rets->size(); ++i) {
                  ctx->set_output(ret_indices[i], (*rets)[i]);
                }
              }
              delete rets;
              VLOG(3) << "Finished remote execution.";
              refcounted_done->Unref();
            });
      }
    }
    refcounted_done->Unref();
  }

  string UniquifyFunctionName(const FunctionLibraryDefinition* function_library,
                              const string& name) {
    for (;; ++suffix_) {
      const string candidate = strings::StrCat(name, "_", suffix_);
      if (function_library->Find(candidate) == nullptr) {
        return candidate;
      }
    }
  }

  Status OptimizeGraph(OpKernelContext* ctx,
                       const gtl::InlinedVector<Node*, 4>& ret_nodes,
                       FunctionLibraryDefinition* flib,
                       const DeviceSet& device_set, Device* cpu_device,
                       std::unique_ptr<Graph>* graph) {
    if (!tensorflow::grappler::MetaOptimizerEnabled(config_proto_)) {
      return Status::OK();
    }

    tensorflow::grappler::GrapplerItem item;

    // Add all available devices so that inlined function can be placed.
    for (const Device* d : device_set.devices()) {
      Status added_device = item.AddDevice(d->name());
      if (!added_device.ok()) VLOG(3) << added_device.error_message();
    }

    // Add fetches so that the graph can be pruned.
    for (Node* node : ret_nodes) {
      item.fetch.push_back(node->name());
    }

    (*graph)->ToGraphDef(&item.graph);

    if (flib) {
      *item.graph.mutable_library() = flib->ToProto();
    }

    tensorflow::GraphDef out_graph;

    tensorflow::grappler::VirtualCluster cluster(&device_set);

    // TODO(nareshmodi): Consider adding and using the more generic GraphOptions
    // proto (which also contain the OptimizerOptions).
    TF_RETURN_IF_ERROR(tensorflow::grappler::RunMetaOptimizer(
        item, config_proto_, cpu_device, &cluster, &out_graph));

    std::unique_ptr<Graph> optimized_graph(new Graph(OpRegistry::Global()));
    TF_RETURN_IF_ERROR(ConvertGraphDefToGraph(
        GraphConstructorOptions(), out_graph, optimized_graph.get()));

    // Copy optimized functions back to the overlay lib.
    if (flib) {
      for (const FunctionDef& fdef : out_graph.library().function()) {
        const string& func_name = fdef.signature().name();
        if (flib->Contains(func_name)) {
          TF_RETURN_IF_ERROR(flib->ReplaceFunction(func_name, fdef));
        } else {
          TF_RETURN_IF_ERROR(flib->AddFunctionDef(fdef));
        }
      }
    }

    *graph = std::move(optimized_graph);

    // The graph conversion sets the requested device names but not the
    // assigned device names. However, since at this point the graph is
    // placed TF expects an assigned device name for every node. Therefore
    // we copy the requested device into the assigned device field.
    for (Node* node : graph->get()->nodes()) {
      node->set_assigned_device_name(node->requested_device());
    }

    return Status::OK();
  }

  NameAttrList func_;
  ConfigProto config_proto_;
  string executor_type_;
  // Contains maps from device names to handles of function partitions, keyed by
  // FunctionLibraryRuntime pointers. (Because this kernel may be instantiated
  // for a stateful op, different invocations of it may use different
  // FLRs. Different device placements of PartitionedCallOp also use different
  // FLRs, and we use this to set the "default" device for the function to
  // PartitionedCallOp's device.)
  gtl::FlatMap<FunctionLibraryRuntime*,
               std::unique_ptr<gtl::FlatMap<string, FHandle>>>
      function_handles_ GUARDED_BY(mu_);
  // Function partitions are added to overlay libraries.
  gtl::FlatMap<FunctionLibraryRuntime*,
               std::unique_ptr<FunctionLibraryDefinition>>
      overlay_libs_ GUARDED_BY(mu_);
  // Map from device name to the indices of the arguments and return values
  // placed on that device. Read-only after the first invocation.
  gtl::FlatMap<string, ArgAndRetIndices> arg_and_ret_indices_;
  // Map from device name to alloc attrs for arguments and return values of the
  // function placed on that device. Read-only after the first invocation.
  gtl::FlatMap<string, ArgAndRetAllocAttrs> arg_and_ret_alloc_attrs_;

  mutex mu_;

  // Used to uniquify function names in `overlay_libs_`.
  uint32 suffix_ = 0;
};
REGISTER_KERNEL_BUILDER(Name("PartitionedCall").Device(DEVICE_CPU),
                        PartitionedCallOp);
REGISTER_KERNEL_BUILDER(Name("StatefulPartitionedCall").Device(DEVICE_CPU),
                        PartitionedCallOp);
REGISTER_KERNEL_BUILDER(Name("PartitionedCall").Device(DEVICE_GPU),
                        PartitionedCallOp);
REGISTER_KERNEL_BUILDER(Name("StatefulPartitionedCall").Device(DEVICE_GPU),
                        PartitionedCallOp);
#if TENSORFLOW_USE_SYCL
REGISTER_KERNEL_BUILDER(Name("PartitionedCall").Device(DEVICE_SYCL),
                        PartitionedCallOp);
REGISTER_KERNEL_BUILDER(Name("StatefulPartitionedCall").Device(DEVICE_SYCL),
                        PartitionedCallOp);
#endif  // TENSORFLOW_USE_SYCL

}  // namespace
}  // namespace tensorflow
