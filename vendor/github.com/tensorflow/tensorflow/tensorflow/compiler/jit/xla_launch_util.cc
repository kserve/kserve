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

#include "tensorflow/compiler/jit/xla_launch_util.h"

#include <memory>

#include "absl/algorithm/container.h"
#include "absl/memory/memory.h"
#include "tensorflow/compiler/jit/defs.h"
#include "tensorflow/compiler/tf2xla/shape_util.h"
#include "tensorflow/compiler/tf2xla/xla_compiler.h"
#include "tensorflow/compiler/xla/client/client_library.h"
#include "tensorflow/compiler/xla/client/local_client.h"
#include "tensorflow/compiler/xla/statusor.h"
#include "tensorflow/core/common_runtime/dma_helper.h"
#include "tensorflow/core/common_runtime/function.h"
#include "tensorflow/core/common_runtime/gpu_device_context.h"
#include "tensorflow/core/framework/allocator.h"
#include "tensorflow/core/framework/node_def_util.h"
#include "tensorflow/core/framework/op.h"
#include "tensorflow/core/framework/op_kernel.h"
#include "tensorflow/core/framework/tensor.h"
#include "tensorflow/core/framework/types.h"
#include "tensorflow/core/util/stream_executor_util.h"

namespace tensorflow {
namespace {
using xla::ScopedShapedBuffer;
using xla::ShapedBuffer;
}  // anonymous namespace

VariableInfo::VariableInfo(int index, Var* var) : index_(index), var_(var) {}
VariableInfo::VariableInfo(VariableInfo&& other)
    : index_(other.index_), var_(other.var_), lock_held_(other.lock_held_) {
  other.index_ = -1;
  other.var_ = nullptr;
}

VariableInfo& VariableInfo::operator=(VariableInfo&& other) {
  index_ = other.index_;
  var_ = other.var_;
  lock_held_ = other.lock_held_;

  other.index_ = -1;
  other.var_ = nullptr;

  return *this;
}

VariableInfo::~VariableInfo() {
  // Release the variable's lock if we hold it. Ensures that the lock is
  // released even on error.  It does not matter in what order we release the
  // locks.
  if (var()) {
    if (lock_held()) {
      var()->mu()->unlock();
    }

    // Unref the variable so it can be released by ResourceManager.
    var()->Unref();
  }
}

// Returns a vector of VaribleInfo instances for the resource variable inputs to
// the kernel with context `ctx`.  The input indices for the resource variable
// inputs are in `variable_indices`.
static Status GetVariableInfosFromCtxInputs(
    OpKernelContext* ctx, absl::Span<const int> variable_indices,
    std::vector<VariableInfo>* result) {
  std::vector<const ResourceHandle*> resource_handles;
  absl::c_transform(
      variable_indices, std::back_inserter(resource_handles),
      [&](int variable_idx) { return &HandleFromInput(ctx, variable_idx); });

  std::vector<std::unique_ptr<Var, core::RefCountDeleter>> variables;
  TF_RETURN_IF_ERROR(LookupResources(ctx, resource_handles, &variables));

  result->clear();
  result->reserve(variable_indices.size());
  for (int i = 0; i < variable_indices.size(); i++) {
    // *Release* the variable because we're going to unref it later in
    // ~VariableInfo.
    Var* variable = variables[i].release();
    result->emplace_back(variable_indices[i], variable);
  }

  return Status::OK();
}

Status LockVariables(absl::Span<VariableInfo> variables) {
  std::vector<int> lock_order(variables.size());
  std::iota(lock_order.begin(), lock_order.end(), 0);

  // VariableInfoComparator orders all empty VariableInfo instances as
  // equivalent so it looks like we may want to stable sort these to maintain a
  // deterministic order between the empty VariableInfo instances.  However
  // since we're sorting by pointer value the sort is pretty non-deterministic
  // anyway so we don't bother using std::stable_sort for now.
  absl::c_sort(lock_order, [&](int a, int b) {
    if (variables[a].var() && variables[b].var()) {
      return variables[a].var()->mu() < variables[b].var()->mu();
    }

    // Move all the empty VariableInfo instances to the end.
    return variables[a].var() != nullptr;
  });

  mutex* prev = nullptr;
  for (int i : lock_order) {
    Var* variable = variables[i].var();
    if (variable == nullptr) {
      // All empty VariableInfo instances are at the end of the order
      // so we're done.
      break;
    }
    mutex* mu = variable->mu();
    if (prev == mu) {
      // It is an error to pass the same variable handle twice to the same XLA
      // cluster because we would not handle variable updates correctly.  Any
      // locks we have already acquired will be released when the VariableInfo
      // objects are destroyed.
      return errors::Internal("Duplicate variable passed to XLA cluster");
    }
    VLOG(4) << "Acquiring lock for variable "
            << reinterpret_cast<void*>(variable);
    mu->lock();
    variables[i].set_lock_held();
    prev = mu;
  }
  VLOG(4) << "Finished acquiring variable locks.";
  return Status::OK();
}

Status SnapshotResourceVariables(OpKernelContext* ctx,
                                 absl::Span<const int> variable_indices,
                                 std::map<int, OptionalTensor>* result) {
  std::vector<VariableInfo> variable_infos;
  TF_RETURN_IF_ERROR(
      GetVariableInfosFromCtxInputs(ctx, variable_indices, &variable_infos));
  TF_RETURN_IF_ERROR(LockVariables(absl::MakeSpan(variable_infos)));

  for (int i = 0; i < variable_indices.size(); i++) {
    if (variable_infos[i].var()) {
      OptionalTensor& tensor = (*result)[variable_indices[i]];
      tensor.name = HandleFromInput(ctx, variable_indices[i]).name();
      tensor.present = true;
      tensor.value = *variable_infos[i].var()->tensor();
    } else {
      (*result)[variable_indices[i]] = OptionalTensor();
    }
  }
  return Status::OK();
}

XlaAllocator::XlaAllocator(const se::Platform* platform, Allocator* wrapped)
    : xla::DeviceMemoryAllocator(platform), wrapped_(wrapped) {}

XlaAllocator::~XlaAllocator() {}

xla::StatusOr<xla::OwningDeviceMemory> XlaAllocator::Allocate(
    int device_ordinal, uint64 size, bool retry_on_failure) {
  AllocationAttributes attrs;
  attrs.no_retry_on_failure = !retry_on_failure;
  void* data = nullptr;
  if (size != 0) {
    data = wrapped_->AllocateRaw(Allocator::kAllocatorAlignment, size, attrs);
    if (data == nullptr) {
      return errors::ResourceExhausted(
          "Out of memory while trying to allocate ", size, " bytes.");
    }
  }
  return xla::OwningDeviceMemory(se::DeviceMemoryBase(data, size),
                                 device_ordinal, this);
}

Status XlaAllocator::Deallocate(int device_ordinal, se::DeviceMemoryBase mem) {
  wrapped_->DeallocateRaw(mem.opaque());
  return Status::OK();
}

XlaComputationLaunchContext::XlaComputationLaunchContext(
    xla::LocalClient* client, xla::DeviceMemoryAllocator* xla_allocator,
    bool allocate_xla_tensors, bool use_multiple_streams)
    : client_(client),
      xla_allocator_(xla_allocator),
      allocate_xla_tensors_(allocate_xla_tensors),
      use_multiple_streams_(use_multiple_streams) {
  if (use_multiple_streams_) {
    CHECK(allocate_xla_tensors_) << "To use multiple streams correctly we must "
                                    "be allocating XLA tensors!";
  }
}

void XlaComputationLaunchContext::PopulateInputs(
    OpKernelContext* ctx, const XlaCompiler::CompilationResult* kernel,
    const std::map<int, OptionalTensor>& variables,
    int missing_ctx_input_prefix) {
  se::Stream* stream =
      ctx->op_device_context() ? ctx->op_device_context()->stream() : nullptr;
  // Build ShapedBuffers that point directly to the Tensor buffers.
  arg_buffers_.reserve(kernel->xla_input_shapes.size() + 1);
  arg_buffers_.resize(kernel->xla_input_shapes.size());
  arg_ptrs_ = std::vector<ShapedBuffer*>(arg_buffers_.size());

  // Pass remaining parameters.
  const Tensor* t;
  for (int i = 0; i < kernel->xla_input_shapes.size(); ++i) {
    int arg_num = kernel->input_mapping[i];
    DCHECK_GE(arg_num, missing_ctx_input_prefix);
    const xla::Shape& shape = kernel->xla_input_shapes[i];
    if (variables.count(arg_num)) {
      t = &(variables.at(arg_num).value);
      CHECK(t);
    } else {
      t = &(ctx->input(arg_num - missing_ctx_input_prefix));
    }

    if (use_multiple_streams_) {
      CHECK(stream) << "Must have a stream available when using XLA tensors!";
      XlaTensor* xla_tensor = XlaTensor::FromTensor(t);
      CHECK(xla_tensor);
      xla_tensor->WaitForDefinitionEventOnStream(stream);
    }

    const xla::Shape on_device_shape =
        client_->backend().transfer_manager()->HostShapeToDeviceShape(shape);
    if (xla::ShapeUtil::IsTuple(on_device_shape)) {
      const XlaTensor* xla_tensor = XlaTensor::FromTensor(t);
      CHECK(xla_tensor && xla_tensor->has_shaped_buffer());
      arg_ptrs_[i] = const_cast<ShapedBuffer*>(&xla_tensor->shaped_buffer());
    } else {
      CHECK(xla::ShapeUtil::Equal(shape, on_device_shape))
          << "On-device shape "
          << xla::ShapeUtil::HumanStringWithLayout(on_device_shape)
          << " not the same as on-host shape "
          << xla::ShapeUtil::HumanStringWithLayout(shape);
      se::DeviceMemoryBase dmem = XlaTensor::DeviceMemoryFromTensor(*t);
      arg_buffers_[i] = absl::make_unique<ShapedBuffer>(
          /*on_host_shape=*/shape, /*on_device_shape=*/shape,
          client_->platform(), client_->default_device_ordinal());
      arg_buffers_[i]->set_buffer(dmem, /*index=*/{});
      arg_ptrs_[i] = arg_buffers_[i].get();
    }
  }
}

Status XlaComputationLaunchContext::PopulateOutputs(
    OpKernelContext* ctx, const XlaCompiler::CompilationResult* kernel,
    ScopedShapedBuffer output, int missing_ctx_input_prefix) {
  se::Stream* stream =
      ctx->op_device_context() ? ctx->op_device_context()->stream() : nullptr;

  // Computation output should always be a tuple.
  if (VLOG_IS_ON(2)) {
    VLOG(2) << "Result tuple shape: " << output.on_host_shape().DebugString();
    VLOG(2) << "Result tuple shape (on device): "
            << output.on_device_shape().DebugString();
  }
  CHECK_EQ(ctx->num_outputs(), kernel->outputs.size());

  // If the on-host-shape isn't a tuple, create a new single-element tuple
  // buffer with a nullptr root index table. This allows the code below to treat
  // output as a tuple unconditionally.
  if (!xla::ShapeUtil::IsTuple(output.on_host_shape())) {
    ShapedBuffer nontuple_buffer = output.release();
    ShapedBuffer buffer(
        xla::ShapeUtil::MakeTupleShape({nontuple_buffer.on_host_shape()}),
        xla::ShapeUtil::MakeTupleShape({nontuple_buffer.on_device_shape()}),
        output.platform(), output.device_ordinal());
    buffer.buffers().CopySubtreeFrom(nontuple_buffer.buffers(),
                                     /*source_base_index=*/{},
                                     /*target_base_index=*/{0});
    output = ScopedShapedBuffer(std::move(buffer), output.memory_allocator());
  }

  std::shared_ptr<se::Event> definition_event;
  if (use_multiple_streams_) {
    definition_event = std::make_shared<se::Event>(stream->parent());
    if (!definition_event->Init()) {
      return errors::Internal("Failed to initialize tensor definition event.");
    }
    stream->ThenRecordEvent(definition_event.get());
  }

  // Copy XLA results to the OpOutputList.
  int output_num = 0;
  for (int i = 0; i < ctx->num_outputs(); ++i) {
    Allocator* allocator = ctx->device()->GetAllocator({});
    if (kernel->outputs[i].is_constant) {
      // Output is a constant.
      const Tensor& const_tensor = kernel->outputs[i].constant_value;
      Tensor* output_tensor;
      const size_t total_bytes = const_tensor.TotalBytes();
      if (stream && total_bytes > 0) {
        // Copy host -> device. (Empty tensors don't have backing buffers.)
        // Manually allocate memory using an XlaTensorBuffer so we can allocate
        // as much memory as the device requires (as given by
        // GetByteSizeRequirement). This avoids XlaTransferManager having to
        // reallocate the device buffer later.
        VLOG(1) << "Constant output tensor on device";

        TF_RETURN_IF_ERROR(
            ctx->allocate_output(i, const_tensor.shape(), &output_tensor));

        Device* device = dynamic_cast<Device*>(ctx->device());
        if (device == nullptr) {
          return errors::Internal("DeviceBase was not a Device.");
        }
        ctx->op_device_context()->CopyCPUTensorToDevice(
            &const_tensor, device, output_tensor,
            [&](Status status) { TF_CHECK_OK(status); });

        if (device->device_type() == DEVICE_GPU) {
          // The GPUDeviceContext enqueues the host->device transfer in a
          // separate stream from the main compute stream. We must ensure the
          // compute stream is synchronized with the host->device transfer
          // stream now otherwise we will create a race condition.
          auto* gpu_device_context =
              static_cast<GPUDeviceContext*>(ctx->op_device_context());
          gpu_device_context->stream()->ThenWaitFor(
              gpu_device_context->host_to_device_stream());
        }
      } else {
        // No copy required.
        ctx->set_output(i, const_tensor);
        output_tensor = ctx->mutable_output(i);
      }
      if (XlaTensor* xla_tensor = XlaTensor::FromTensor(output_tensor)) {
        xla_tensor->set_host_tensor(const_tensor);
      }
    } else {
      const TensorShape& shape = kernel->outputs[i].shape;
      const DataType& type = kernel->outputs[i].type;
      VLOG(2) << "Retval " << i << " shape " << shape.DebugString() << " type "
              << DataTypeString(type);
      if (type == DT_RESOURCE) {
        TF_RET_CHECK(kernel->outputs[i].input_index >= 0)
            << "Invalid input for outputs " << i;
        ctx->set_output(i, ctx->input(kernel->outputs[i].input_index));
      } else {
        se::DeviceMemoryBase buffer = output.buffer({output_num});
        if (allocate_xla_tensors_) {
          Tensor* output_tensor;
          TF_RETURN_IF_ERROR(ctx->allocate_output(i, shape, &output_tensor));
          XlaTensor* xla_tensor = XlaTensor::FromTensor(output_tensor);
          if (xla_tensor) {
            xla_tensor->set_shaped_buffer(output.TakeSubTree({output_num}));
            if (use_multiple_streams_) {
              xla_tensor->ResetDefinitionEvent(definition_event, stream);
            }
          } else {
            // xla_tensor wasn't valid, which must mean this is a zero-element
            // tensor.
            CHECK_EQ(output_tensor->TotalBytes(), 0);
          }
        } else {
          Tensor output_tensor = XlaTensorBuffer::MakeTensor(
              ctx->expected_output_dtype(i), shape, buffer, allocator);
          output.set_buffer(xla::OwningDeviceMemory(), {output_num});
          ctx->set_output(i, output_tensor);
        }
        ++output_num;
      }
    }

    if (VLOG_IS_ON(3)) {
      VLOG(3) << ctx->mutable_output(i)->DebugString();
    }
  }

  // Apply variable updates, if any.
  VLOG(2) << "Applying variable updates";
  std::vector<VariableInfo> variable_infos;
  variable_infos.reserve(kernel->resource_updates.size());

  for (int i = 0; i < kernel->resource_updates.size(); ++i) {
    const XlaCompiler::ResourceUpdate& write = kernel->resource_updates[i];
    int actual_input_index = write.input_index - missing_ctx_input_prefix;
    if (actual_input_index < 0 || actual_input_index >= ctx->num_inputs()) {
      return errors::Internal("Invalid input index for variable write.");
    }

    // TODO(b/35625933): tensorflow::Var should contain a PersistentTensor,
    // not a Tensor.
    Var* variable = nullptr;
    TF_RETURN_IF_ERROR(LookupOrCreateResource<Var>(
        ctx, HandleFromInput(ctx, actual_input_index), &variable,
        [&write](Var** ptr) {
          *ptr = new Var(write.type);
          return Status::OK();
        }));
    variable_infos.emplace_back(actual_input_index, variable);
  }

  TF_RETURN_IF_ERROR(LockVariables(absl::MakeSpan(variable_infos)));

  for (int i = 0; i < kernel->resource_updates.size(); ++i) {
    Allocator* allocator = ctx->device()->GetAllocator({});
    const XlaCompiler::ResourceUpdate& write = kernel->resource_updates[i];

    if (variable_infos[i].var()->tensor()->dtype() != write.type) {
      return errors::Internal("Mismatched type in variable write");
    }

    if (allocate_xla_tensors_) {
      Tensor output_tensor;
      TF_RETURN_IF_ERROR(
          ctx->allocate_temp(write.type, write.shape, &output_tensor));
      if (write.shape.num_elements() > 0) {
        XlaTensor* xla_tensor = XlaTensor::FromTensor(&output_tensor);
        CHECK(xla_tensor);
        xla_tensor->set_shaped_buffer(output.TakeSubTree({output_num}));
        if (use_multiple_streams_) {
          xla_tensor->ResetDefinitionEvent(definition_event, stream);
        }
      }
      *variable_infos[i].var()->tensor() = output_tensor;
    } else {
      se::DeviceMemoryBase buffer = output.buffer({output_num});
      output.set_buffer(xla::OwningDeviceMemory(), {output_num});
      Tensor output_tensor = XlaTensorBuffer::MakeTensor(
          write.type, write.shape, buffer, allocator);
      *variable_infos[i].var()->tensor() = output_tensor;
    }
    ++output_num;
  }
  return Status::OK();
}

Status XlaComputationLaunchContext::BuildXlaCompilerArguments(
    const std::map<int, Tensor>& constant_args,
    const std::map<int, OptionalTensor>& variable_args, OpKernelContext* ctx,
    std::vector<XlaCompiler::Argument>* args) {
  args->resize(ctx->num_inputs());

  for (int64 input_num = 0; input_num < ctx->num_inputs(); ++input_num) {
    XlaCompiler::Argument& arg = (*args)[input_num];
    if (constant_args.count(input_num) > 0) {
      // Handles compile-time constants.
      const Tensor& input = constant_args.at(input_num);
      TF_RET_CHECK(input.dtype() != DT_RESOURCE);
      arg.kind = XlaCompiler::Argument::kConstant;
      arg.type = input.dtype();
      arg.shape = input.shape();
      arg.constant_value = input;
    } else if (variable_args.count(input_num) == 0) {
      // Handles the non-constant arguments.
      const Tensor& input = ctx->input(input_num);
      TF_RET_CHECK(input.dtype() != DT_RESOURCE);
      if (input.NumElements() > 0) {
        arg.kind = XlaCompiler::Argument::kParameter;
      } else {
        arg.kind = XlaCompiler::Argument::kConstant;
        arg.constant_value = input;
      }
      arg.type = input.dtype();
      arg.shape = input.shape();
    } else {
      // Handles resource variables.
      const Tensor& input = ctx->input(input_num);
      TF_RET_CHECK(input.dtype() == DT_RESOURCE);
      const OptionalTensor& variable = variable_args.at(input_num);
      arg.name = variable.name;
      arg.kind = XlaCompiler::Argument::kResource;
      arg.resource_kind = XlaResource::kVariable;
      if (variable.present) {
        const Tensor& value = variable.value;
        arg.type = value.dtype();
        arg.shape = value.shape();
        arg.initialized = true;
      } else {
        // The values of uninitialized variables are not passed as inputs, since
        // they are meaningless. However, it is legal to assign to a resource
        // variable for the first time inside the XLA computation, so we do
        // permit uninitialized variables.
        arg.initialized = false;
        arg.type = DT_INVALID;
        arg.shape = TensorShape();
      }
    }
  }

  return Status::OK();
}

}  // namespace tensorflow
