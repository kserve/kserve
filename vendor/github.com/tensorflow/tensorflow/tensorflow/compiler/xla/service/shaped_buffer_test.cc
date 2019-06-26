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

#include "tensorflow/compiler/xla/service/shaped_buffer.h"

#include "absl/memory/memory.h"
#include "tensorflow/compiler/xla/service/device_memory_allocator.h"
#include "tensorflow/compiler/xla/service/platform_util.h"
#include "tensorflow/compiler/xla/shape_util.h"
#include "tensorflow/compiler/xla/test.h"
#include "tensorflow/core/platform/stream_executor_no_cuda.h"
#include "tensorflow/core/platform/test_benchmark.h"
#include "tensorflow/core/util/ptr_util.h"

namespace xla {
namespace {

TEST(ShapedBufferTest, ScopedShapeBufferAsShapedBufferB71629047) {
  TF_ASSERT_OK_AND_ASSIGN(auto platforms,
                          xla::PlatformUtil::GetSupportedPlatforms());
  ASSERT_FALSE(platforms.empty());
  auto* platform = platforms[0];
  TF_ASSERT_OK_AND_ASSIGN(auto executors,
                          xla::PlatformUtil::GetStreamExecutors(platform));
  xla::StreamExecutorMemoryAllocator allocator(platform, executors);
  const xla::Shape shape = xla::ShapeUtil::MakeShape(xla::F32, {});
  const int kDeviceOrdinal = 0;
  auto scoped_buffer = absl::make_unique<xla::ScopedShapedBuffer>(
      shape, shape, &allocator, kDeviceOrdinal);
  std::unique_ptr<xla::ShapedBuffer> buffer = std::move(scoped_buffer);
  buffer = nullptr;
}

class TestAllocator : public DeviceMemoryAllocator {
 public:
  TestAllocator()
      : DeviceMemoryAllocator(PlatformUtil::GetDefaultPlatform().ValueOrDie()) {
  }

  ~TestAllocator() override {
    if (!allocations_.empty()) {
      ADD_FAILURE() << "Some allocations not freed!";
    }
  }

  // Pull in two-arg overload of Allocate.
  using DeviceMemoryAllocator::Allocate;

  StatusOr<OwningDeviceMemory> Allocate(int device_ordinal, uint64 size,
                                        bool /*retry_on_failure*/) override {
    // By contract, we must return null if size == 0.
    if (size == 0) {
      return OwningDeviceMemory();
    }
    void* buf = malloc(size);
    allocations_.insert({device_ordinal, buf});
    return OwningDeviceMemory(se::DeviceMemoryBase(buf, size), device_ordinal,
                              this);
  }

  Status Deallocate(int device_ordinal, se::DeviceMemoryBase mem) override {
    if (mem.is_null()) {
      return Status::OK();
    }

    auto it = allocations_.find({device_ordinal, mem.opaque()});
    if (it == allocations_.end()) {
      ADD_FAILURE() << "Allocation not found (double free?)";
    } else {
      free(mem.opaque());
      allocations_.erase(it);
    }
    return Status::OK();
  }

  bool AllowsAsynchronousDeallocation() const override { return false; }

 private:
  std::set<std::pair</*device_ordinal*/ int64, void*>> allocations_;
};

TEST(ScopedShapedBufferTest, TestMoveAssignmentOperator) {
  Shape s = ShapeUtil::MakeShape(F32, {1});
  TestAllocator allocator;
  ScopedShapedBuffer sb1(s, s, &allocator, /*device_ordinal=*/0);
  sb1.set_buffer(
      allocator.Allocate(/*device_ordinal=*/0, /*size=*/42).ValueOrDie(),
      /*index=*/{});

  ScopedShapedBuffer sb2(s, s, &allocator, /*device_ordinal=*/1);
  sb2.set_buffer(
      allocator.Allocate(/*device_ordinal=*/1, /*size=*/10).ValueOrDie(),
      /*index=*/{});

  sb1 = std::move(sb2);

  // TestAllocator's destructor checks that all memory was freed.
}

TEST(ScopedShapedBufferTest, TestTakeSubTree) {
  TestAllocator allocator;

  Shape s = ShapeUtil::MakeShape(F32, {1});
  s = xla::ShapeUtil::MakeTupleShape(std::vector<xla::Shape>(2, s));
  s = xla::ShapeUtil::MakeTupleShape(std::vector<xla::Shape>(3, s));

  ScopedShapedBuffer sb(s, s, &allocator, /*device_ordinal=*/0);
  sb.buffers().ForEachMutableElement(
      [&](const xla::ShapeIndex& index, se::DeviceMemoryBase* buffer) {
        TF_ASSERT_OK_AND_ASSIGN(
            OwningDeviceMemory m,
            allocator.Allocate(/*device_ordinal=*/0, /*size=*/77));
        *buffer = m.Forget();
      });
  ShapeTree<se::DeviceMemoryBase> buffers = sb.buffers();

  // Takes a subtree out of 'sb', and verifies the buffers are as expected.
  xla::ShapeIndex subtree_index = {1};
  ScopedShapedBuffer output = sb.TakeSubTree(subtree_index);

  output.buffers().ForEachElement([&](const xla::ShapeIndex& sub_index,
                                      const se::DeviceMemoryBase& buffer) {
    xla::ShapeIndex orig_index = subtree_index;
    for (int i : sub_index) {
      orig_index.push_back(i);
    }
    EXPECT_TRUE(buffers.find(orig_index)->second.IsSameAs(buffer));
  });
  sb.buffers().ForEachElement(
      [&](const xla::ShapeIndex& index, const se::DeviceMemoryBase& buffer) {
        if (ShapeIndexView(index).StartsWith(subtree_index)) {
          EXPECT_TRUE(buffer.is_null());
        } else {
          EXPECT_TRUE(buffers.find(index)->second.IsSameAs(buffer));
        }
      });
}

// Test TakeSubTree with different depths (depth of ShapeTree) and fan-outs
// (cardinality of each non-leaf node's children).
void BM_TakeSubTree(int iters, int depth, int fan_out) {
  tensorflow::testing::StopTiming();
  TestAllocator allocator;
  xla::Shape shape = xla::ShapeUtil::MakeShape(xla::F32, {32, 64, 128});
  for (int i = 0; i < depth; ++i) {
    std::vector<xla::Shape> shapes(fan_out, shape);
    shape = xla::ShapeUtil::MakeTupleShape(shapes);
  }
  xla::ScopedShapedBuffer shaped_buffer(shape, shape, /*allocator=*/&allocator,
                                        /*device_ordinal=*/0);
  tensorflow::testing::StartTiming();
  for (int i = 0; i < iters; ++i) {
    // Extract a buffer from approximately the middle of the first level of the
    // tree.
    (void)shaped_buffer.TakeSubTree(/*index=*/{fan_out / 2}).release();
  }
  tensorflow::testing::StopTiming();
}

BENCHMARK(BM_TakeSubTree)
    ->ArgPair(1, 4)
    ->ArgPair(1, 8)
    ->ArgPair(1, 32)
    ->ArgPair(1, 64)
    ->ArgPair(1, 128)
    ->ArgPair(1, 256)
    ->ArgPair(1, 512)
    ->ArgPair(2, 4)
    ->ArgPair(2, 8)
    ->ArgPair(2, 32)
    ->ArgPair(2, 64)
    ->ArgPair(2, 128);

}  // anonymous namespace
}  // namespace xla
