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

#include "tensorflow/compiler/xla/service/gpu/gpu_transfer_manager.h"

#include <string>
#include <utility>
#include <vector>

#include "absl/memory/memory.h"
#include "llvm/IR/DataLayout.h"
#include "tensorflow/compiler/xla/literal.h"
#include "tensorflow/compiler/xla/literal_util.h"
#include "tensorflow/compiler/xla/service/gpu/nvptx_compiler.h"
#include "tensorflow/compiler/xla/service/gpu/outfeed_manager.h"
#include "tensorflow/compiler/xla/shape_util.h"
#include "tensorflow/compiler/xla/status_macros.h"
#include "tensorflow/compiler/xla/statusor.h"
#include "tensorflow/compiler/xla/types.h"
#include "tensorflow/compiler/xla/util.h"
#include "tensorflow/compiler/xla/xla_data.pb.h"
#include "tensorflow/core/lib/core/errors.h"
#include "tensorflow/core/lib/gtl/cleanup.h"
#include "tensorflow/core/platform/logging.h"
#include "tensorflow/core/platform/stream_executor_no_cuda.h"

namespace xla {
namespace gpu {

// TODO(b/30467474) Once GPU infeed implementation settles, consider
// folding back the cpu and gpu infeed implementations into a generic
// one if possible.
GpuTransferManager::GpuTransferManager(se::Platform::Id id,
                                       unsigned pointer_size)
    : GenericTransferManager(id, pointer_size) {}

Status GpuTransferManager::TransferLiteralToInfeed(
    se::StreamExecutor* executor, const LiteralSlice& literal) {
  const Shape& shape = literal.shape();
  VLOG(2) << "Transferring literal to infeed with shape: "
          << ShapeUtil::HumanString(shape);

  // For a tuple, we transfer each of its elements to the device and
  // enqueue the resulting destination device addresses with the
  // infeed manager.
  ShapeTree<InfeedBuffer> buffer_tree(shape);

  TF_RETURN_IF_ERROR(ShapeUtil::ForEachSubshapeWithStatus(
      shape, [&](const Shape& literal_subshape, const ShapeIndex& index) {
        if (ShapeUtil::IsArray(literal_subshape)) {
          int64 tuple_element_size = GetByteSizeRequirement(literal_subshape);
          TF_ASSIGN_OR_RETURN(
              *buffer_tree.mutable_element(index),
              TransferBufferToInfeedInternal(executor, tuple_element_size,
                                             literal.untyped_data(index)));
        }
        return Status::OK();
      }));

  return EnqueueBuffersToInfeed(executor, std::move(buffer_tree));
}

Status GpuTransferManager::EnqueueBuffersToInfeed(
    se::StreamExecutor* executor, ShapeTree<InfeedBuffer> buffers) {
  gpu::InfeedManager* infeed_manager = gpu::GetOrCreateInfeedManager();
  se::Stream* stream = infeed_manager->GetStream(executor);

  // TODO(b/30467474): Since this stream is shared across different
  // infeed requests, blocking on the stream might be
  // heavy-handed. Figure out if finer-grained acknowledgement is
  // possible.
  Status block_status = stream->BlockHostUntilDone();
  if (!block_status.ok()) {
    return InternalError("Failed to complete data transfer on stream %p: %s",
                         stream, block_status.error_message());
  }

  infeed_manager->EnqueueDestination(std::move(buffers));

  VLOG(2) << "Infeed data transferred";

  return Status::OK();
}

StatusOr<InfeedBuffer> GpuTransferManager::TransferBufferToInfeedInternal(
    se::StreamExecutor* executor, int64 size, const void* source) {
  if (size > std::numeric_limits<int32>::max()) {
    return InvalidArgument("Infeed shape is too large: needs %d bytes", size);
  }

  if (size == 0) {
    return InvalidArgument("Infeed shape needs 0 bytes");
  }

  gpu::InfeedManager* infeed_manager = gpu::GetOrCreateInfeedManager();
  se::Stream* stream = infeed_manager->GetStream(executor);
  if (stream == nullptr) {
    return InternalError("Failed to obtain a stream");
  }

  InfeedBuffer buffer(executor, size);
  stream->ThenMemcpy(buffer.device_memory(), source, size);

  VLOG(2) << "Queued infeed data on stream " << stream;

  return std::move(buffer);
}

static void ShapeTreeToLiteral(
    ShapeTree<std::unique_ptr<gpu::OutfeedBuffer>>* shape_tree) {
  // This is a struct instead of a lambda for std::function-free recursion.
  struct Helper {
    static void helper(
        ShapeTree<std::unique_ptr<gpu::OutfeedBuffer>>* shape_tree,
        ShapeIndex* index) {
      const Shape& shape = ShapeUtil::GetSubshape(shape_tree->shape(), *index);
      if (ShapeUtil::IsArray(shape)) {
        (*shape_tree->mutable_element(*index))->WaitUntilAvailable();
        return;
      }

      CHECK(ShapeUtil::IsTuple(shape))
          << ShapeUtil::HumanStringWithLayout(shape);
      const int64 tuple_element_count = ShapeUtil::TupleElementCount(shape);
      index->push_back(0);
      for (int64 i = 0; i < tuple_element_count; ++i) {
        index->back() = i;
        helper(shape_tree, index);
      }
      index->pop_back();
    }
  };
  ShapeIndex index;
  Helper::helper(shape_tree, &index);
}

Status GpuTransferManager::TransferLiteralFromOutfeed(
    se::StreamExecutor* /*executor*/, const Shape& literal_shape,
    MutableBorrowingLiteral literal) {
  ShapeTree<std::unique_ptr<gpu::OutfeedBuffer>> outfeed_buffers(
      &literal_shape);

  // First create a tree of literal buffers that the device can write to.
  outfeed_buffers.ForEachMutableElement(
      [&](const ShapeIndex& index,
          std::unique_ptr<gpu::OutfeedBuffer>* buffer) {
        const Shape& shape = ShapeUtil::GetSubshape(literal_shape, index);
        // Do not transfer tuple index buffers.
        if (ShapeUtil::IsTuple(shape)) {
          return;
        }
        *buffer = absl::make_unique<gpu::OutfeedBuffer>(
            GetByteSizeRequirement(shape));
        (*buffer)->set_destination(
            absl::make_unique<MutableBorrowingLiteral>(literal, index));
      });

  // Give the tree of buffers to the outfeed mananger. The device will fill it
  // while we're waiting for it below.
  gpu::OutfeedManager* outfeed_manager = gpu::GetOrCreateOutfeedManager();
  outfeed_manager->EnqueueDestination(&outfeed_buffers);

  // Now wait for the tree of buffers are written.
  ShapeTreeToLiteral(&outfeed_buffers);
  return Status::OK();
}

}  // namespace gpu
}  // namespace xla

static std::unique_ptr<xla::TransferManager> CreateNVPTXTransferManager() {
  return absl::make_unique<xla::gpu::GpuTransferManager>(
      /*id=*/stream_executor::cuda::kCudaPlatformId,
      /*pointer_size=*/llvm::DataLayout(xla::gpu::NVPTXCompiler::kDataLayout)
          .getPointerSize(0 /* default address space */));
}

static bool InitModule() {
  xla::TransferManager::RegisterTransferManager(
      stream_executor::cuda::kCudaPlatformId, &CreateNVPTXTransferManager);
  return true;
}
static bool module_initialized = InitModule();
