/* Copyright 2015 The TensorFlow Authors. All Rights Reserved.

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

#include "tensorflow/core/framework/allocator.h"

#include <algorithm>
#include <vector>

#include "tensorflow/core/platform/logging.h"
#include "tensorflow/core/platform/test.h"
#include "tensorflow/core/platform/test_benchmark.h"

namespace tensorflow {

static void CheckStats(Allocator* a, int64 num_allocs, int64 bytes_in_use,
                       int64 max_bytes_in_use, int64 max_alloc_size) {
  AllocatorStats stats;
  a->GetStats(&stats);
  LOG(INFO) << "Alloc stats: \n" << stats.DebugString();
#if defined(PLATFORM_GOOGLE) && defined(NDEBUG)
  // NOTE: allocator stats expectation depends on the system malloc,
  // and can vary as that changes.
  static const int64 kSlop = 5 * 1024;
  EXPECT_GT(stats.bytes_in_use, bytes_in_use - kSlop);
  EXPECT_LT(stats.bytes_in_use, bytes_in_use + kSlop);
  EXPECT_GT(stats.max_bytes_in_use, max_bytes_in_use - kSlop);
  EXPECT_LT(stats.max_bytes_in_use, max_bytes_in_use + kSlop);
  EXPECT_EQ(stats.num_allocs, num_allocs);
  EXPECT_EQ(stats.max_alloc_size, max_alloc_size);
#endif
}

TEST(AllocatorAttributesTest, AllCombos) {
  for (bool on_host : {false, true}) {
    for (bool nic_compatible : {false, true}) {
      for (bool gpu_compatible : {false, true}) {
        AllocatorAttributes aa;
        aa.set_on_host(on_host);
        aa.set_nic_compatible(nic_compatible);
        aa.set_gpu_compatible(gpu_compatible);
        EXPECT_EQ(on_host, aa.on_host());
        EXPECT_EQ(nic_compatible, aa.nic_compatible());
        EXPECT_EQ(gpu_compatible, aa.gpu_compatible());
      }
    }
  }
}

TEST(AllocatorAttributesTest, IsEqualOrLessRestrictiveThan) {
  AllocatorAttributes a, b;
  EXPECT_TRUE(a.IsEqualOrLessRestrictiveThan(b));
  EXPECT_TRUE(a.IsEqualOrLessRestrictiveThan(a));
  EXPECT_TRUE(b.IsEqualOrLessRestrictiveThan(b));

  b.set_gpu_compatible(true);
  // The set of flags in b is not a subset of those in a.
  EXPECT_TRUE(a.IsEqualOrLessRestrictiveThan(b));
  EXPECT_FALSE(b.IsEqualOrLessRestrictiveThan(a));
  EXPECT_TRUE(a.IsEqualOrLessRestrictiveThan(a));
  EXPECT_TRUE(b.IsEqualOrLessRestrictiveThan(b));

  a.set_nic_compatible(true);
  // Neither a nor b is a subset of the other.
  EXPECT_FALSE(a.IsEqualOrLessRestrictiveThan(b));
  EXPECT_FALSE(b.IsEqualOrLessRestrictiveThan(a));

  a.set_gpu_compatible(true);
  // The set of flags in b is a proper subset of those in a.
  EXPECT_TRUE(b.IsEqualOrLessRestrictiveThan(a));
  EXPECT_FALSE(a.IsEqualOrLessRestrictiveThan(b));
}

TEST(CPUAllocatorTest, Simple) {
  EnableCPUAllocatorStats(true);
  Allocator* a = cpu_allocator();
  std::vector<void*> ptrs;
  for (int s = 1; s < 1024; s++) {
    void* raw = a->AllocateRaw(1, s);
    ptrs.push_back(raw);
  }
  std::sort(ptrs.begin(), ptrs.end());
  CheckStats(a, 1023, 552640, 552640, 1024);
  for (size_t i = 0; i < ptrs.size(); i++) {
    if (i > 0) {
      CHECK_NE(ptrs[i], ptrs[i - 1]);  // No dups
    }
    a->DeallocateRaw(ptrs[i]);
  }
  CheckStats(a, 1023, 0, 552640, 1024);
  float* t1 = a->Allocate<float>(1024);
  double* t2 = a->Allocate<double>(1048576);
  CheckStats(a, 1025, 1048576 * sizeof(double) + 1024 * sizeof(float),
             1048576 * sizeof(double) + 1024 * sizeof(float),
             1048576 * sizeof(double));

  a->Deallocate(t1, 1024);
  a->Deallocate(t2, 1048576);

  CheckStats(a, 1025, 0, 1048576 * sizeof(double) + 1024 * sizeof(float),
             1048576 * sizeof(double));
  a->ClearStats();
  CheckStats(a, 0, 0, 0, 0);
  EnableCPUAllocatorStats(false);
}

// Define a struct that we will use to observe behavior in the unit tests
struct TestStruct {
  int x;  // not used just want to make sure sizeof(TestStruct) > 1
};

TEST(CPUAllocatorTest, CheckStructSize) { CHECK_GT(sizeof(TestStruct), 1); }

TEST(CPUAllocatorTest, AllocateOverflowMaxSizeT) {
  Allocator* a = cpu_allocator();

  // The maximum size_t value will definitely overflow.
  size_t count_to_allocate = std::numeric_limits<size_t>::max();
  TestStruct* const test_pointer = a->Allocate<TestStruct>(count_to_allocate);

  CHECK_EQ(test_pointer, reinterpret_cast<TestStruct*>(NULL));
}

TEST(CPUAllocatorTest, AllocateOverflowSmallest) {
  Allocator* a = cpu_allocator();

  // count_to_allocate is the smallest count that will cause overflow.
  const size_t count_to_allocate =
      (std::numeric_limits<size_t>::max() / sizeof(TestStruct)) + 1;
  TestStruct* const test_pointer = a->Allocate<TestStruct>(count_to_allocate);

  CHECK_EQ(test_pointer, reinterpret_cast<TestStruct*>(NULL));
}

TEST(CPUAllocatorTest, Sizes) {
  Allocator* a = cpu_allocator();

  EXPECT_EQ(false, a->TracksAllocationSizes());
}

namespace {

AllocatorAttributes DeviceAllocatorAttribute() {
  AllocatorAttributes attr;
  attr.value |= (0x1 << 24);
  return attr;
}

bool HasDeviceAllocatorAttribute(const AllocatorAttributes& attr) {
  return attr.value & (0x1 << 24);
}

}  // namespace

TEST(CustomAllocatorAttributes, TestSetterAndGetter) {
  AllocatorAttributes attr = DeviceAllocatorAttribute();
  EXPECT_TRUE(HasDeviceAllocatorAttribute(attr));
  EXPECT_FALSE(HasDeviceAllocatorAttribute(AllocatorAttributes()));
}

static void BM_Allocation(int iters, int arg) {
  Allocator* a = cpu_allocator();
  // Exercise a few different allocation sizes
  std::vector<int> sizes = {256, 4096, 16384, 524288, 512, 1048576};
  int size_index = 0;

  if (arg) EnableCPUAllocatorStats(true);
  while (--iters > 0) {
    int bytes = sizes[size_index++ % sizes.size()];
    void* p = a->AllocateRaw(1, bytes);
    a->DeallocateRaw(p);
  }
  if (arg) EnableCPUAllocatorStats(false);
}
BENCHMARK(BM_Allocation)->Arg(0)->Arg(1);

}  // namespace tensorflow
