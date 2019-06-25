/* Copyright 2016 The TensorFlow Authors. All Rights Reserved.

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

#include <algorithm>

#include "tensorflow/core/common_runtime/device.h"
#include "tensorflow/core/common_runtime/device_factory.h"
#include "tensorflow/core/common_runtime/executor.h"
#include "tensorflow/core/common_runtime/kernel_benchmark_testlib.h"
#include "tensorflow/core/common_runtime/process_util.h"
#include "tensorflow/core/common_runtime/step_stats_collector.h"
#include "tensorflow/core/framework/op.h"
#include "tensorflow/core/framework/rendezvous.h"
#include "tensorflow/core/framework/step_stats.pb.h"
#include "tensorflow/core/framework/versions.pb.h"
#include "tensorflow/core/graph/graph_constructor.h"
#include "tensorflow/core/lib/core/status_test_util.h"
#include "tensorflow/core/lib/random/simple_philox.h"
#include "tensorflow/core/lib/strings/strcat.h"
#include "tensorflow/core/platform/logging.h"
#include "tensorflow/core/platform/test.h"
#include "tensorflow/core/platform/test_benchmark.h"
#include "tensorflow/core/platform/tracing.h"
#include "tensorflow/core/public/session_options.h"

namespace tensorflow {

class ExecutorTest : public ::testing::Test {
 protected:
  ExecutorTest()
      : device_(DeviceFactory::NewDevice("CPU", {},
                                         "/job:localhost/replica:0/task:0")),

        step_stats_collector_(&step_stats_) {
    SessionOptions options;
    thread_pool_ = ComputePool(options);
  }

  ~ExecutorTest() override {
    // There should always be exactly one Ref left on the Rendezvous
    // when the test completes.
    CHECK(rendez_->Unref());
    delete exec_;
  }

  // Resets executor_ with a new executor based on a graph 'gdef'.
  void Create(std::unique_ptr<const Graph> graph) {
    const int version = graph->versions().producer();
    LocalExecutorParams params;
    params.device = device_.get();
    params.create_kernel = [this, version](const NodeDef& ndef,
                                           OpKernel** kernel) {
      return CreateNonCachedKernel(device_.get(), nullptr, ndef, version,
                                   kernel);
    };
    params.delete_kernel = [](OpKernel* kernel) {
      DeleteNonCachedKernel(kernel);
    };
    delete exec_;
    TF_CHECK_OK(NewLocalExecutor(params, std::move(graph), &exec_));
    runner_ = [this](std::function<void()> fn) { thread_pool_->Schedule(fn); };
    rendez_ = NewLocalRendezvous();
  }

  Status Run(Rendezvous* rendez) {
    Executor::Args args;
    args.rendezvous = rendez;
    args.stats_collector = &step_stats_collector_;
    args.runner = runner_;
    return exec_->Run(args);
  }

  thread::ThreadPool* thread_pool_ = nullptr;
  std::unique_ptr<Device> device_;
  Executor* exec_ = nullptr;
  StepStatsCollector step_stats_collector_;
  StepStats step_stats_;
  Executor::Args::Runner runner_;
  Rendezvous* rendez_ = nullptr;
};

// A float val -> Tensor<float>
Tensor V(const float val) {
  Tensor tensor(DT_FLOAT, TensorShape({}));
  tensor.scalar<float>()() = val;
  return tensor;
}

// A int32 val -> Tensor<int32>
Tensor VI(const int32 val) {
  Tensor tensor(DT_INT32, TensorShape({}));
  tensor.scalar<int32>()() = val;
  return tensor;
}

// A bool val -> Tensor<bool>
Tensor VB(const bool val) {
  Tensor tensor(DT_BOOL, TensorShape({}));
  tensor.scalar<bool>()() = val;
  return tensor;
}

// A double val -> Tensor<double>
Tensor VD(const double val) {
  Tensor tensor(DT_DOUBLE, TensorShape({}));
  tensor.scalar<double>()() = val;
  return tensor;
}

// Tensor<float> -> a float val.
float V(const Tensor& tensor) {
  CHECK_EQ(tensor.dtype(), DT_FLOAT);
  CHECK(TensorShapeUtils::IsScalar(tensor.shape()));
  return tensor.scalar<float>()();
}

static uint64 kIncarnation = 1;  // Uses in following tests.

Rendezvous::ParsedKey Key(const string& sender, const uint64 incarnation,
                          const string& receiver, const string& name) {
  Rendezvous::ParsedKey result;
  CHECK(
      Rendezvous::ParseKey(Rendezvous::CreateKey(sender, incarnation, receiver,
                                                 name, FrameAndIter(0, 0)),
                           &result)
          .ok());
  return result;
}

#define ALICE "/job:j/replica:0/task:0/cpu:0"
#define BOB "/job:j/replica:0/task:0/device:GPU:0"

TEST_F(ExecutorTest, SimpleAdd) {
  // c = a + b
  std::unique_ptr<Graph> g(new Graph(OpRegistry::Global()));
  auto in0 = test::graph::Recv(g.get(), "a", "float", ALICE, 1, BOB);
  auto in1 = test::graph::Recv(g.get(), "b", "float", ALICE, 1, BOB);
  auto tmp = test::graph::Add(g.get(), in0, in1);
  test::graph::Send(g.get(), tmp, "c", BOB, 1, ALICE);
  Create(std::move(g));
  Rendezvous::Args args;
  TF_ASSERT_OK(rendez_->Send(Key(ALICE, kIncarnation, BOB, "a"), args, V(1.0),
                             false));  // in0 = 1.0
  TF_ASSERT_OK(rendez_->Send(Key(ALICE, kIncarnation, BOB, "b"), args, V(1.0),
                             false));  // in1 = 1.0
  TF_ASSERT_OK(Run(rendez_));
  Tensor out = V(-1);
  bool is_dead = false;
  TF_ASSERT_OK(
      rendez_->Recv(Key(BOB, kIncarnation, ALICE, "c"), args, &out, &is_dead));
  EXPECT_EQ(2.0, V(out));  // out = 1.0 + 1.0 = 2.0
}

TEST_F(ExecutorTest, SelfAdd) {
  // v0 <- a
  // v1 = v0 + v0
  // v2 = v1 + v1
  // ... ...
  // v10 = v9 + v9
  //
  // b <- v10
  // All nodes are executed by one thread.
  std::unique_ptr<Graph> g(new Graph(OpRegistry::Global()));
  auto v = test::graph::Recv(g.get(), "a", "float", ALICE, 1, BOB);
  const int N = 10;
  for (int i = 1; i <= N; ++i) {
    v = test::graph::Add(g.get(), v, v);
  }
  // out <- v10
  test::graph::Send(g.get(), v, "b", BOB, 1, ALICE);
  Create(std::move(g));
  Rendezvous::Args args;
  // a = 1.0
  TF_ASSERT_OK(
      rendez_->Send(Key(ALICE, kIncarnation, BOB, "a"), args, V(1.0), false));
  TF_ASSERT_OK(Run(rendez_));
  Tensor out = V(-1);
  bool is_dead = false;
  TF_ASSERT_OK(
      rendez_->Recv(Key(BOB, kIncarnation, ALICE, "b"), args, &out, &is_dead));
  EXPECT_EQ(1024.0, V(out));  // b=v10=2*v9=4*v8=...=1024*a=1024.0
}

// Builds a graph which adds N copies of one variable "in". I.e.,
//     a + a + a + ... + a
// The returned graph is parenthesized ramdonly. I.e.,
//     a + ((a + a) + a)
//     (a + a) + (a + a)
//     ((a + a) + a) + a
// are all possibly generated.
void BuildTree(int N, Graph* g) {
  CHECK_GT(N, 1);
  // A single input node "in".
  auto in = test::graph::Recv(g, "a", "float", ALICE, 1, BOB);
  std::vector<Node*> nodes;
  int i = 0;
  // Duplicate "in" N times. Each copies is named as l0, l1, l2, ....
  for (; i < N; ++i) {
    nodes.push_back(test::graph::Identity(g, in, 0));
  }
  random::PhiloxRandom philox(testing::RandomSeed(), 17);
  random::SimplePhilox rnd(&philox);
  while (nodes.size() > 1) {
    // Randomly pick two from nodes and add them. The resulting node
    // is named lik n10, n11, .... and is put back into "nodes".
    int x = rnd.Uniform(nodes.size());
    auto in0 = nodes[x];
    nodes[x] = nodes.back();
    nodes.resize(nodes.size() - 1);
    x = rnd.Uniform(nodes.size());
    auto in1 = nodes[x];
    // node = in0 + in1.
    nodes[x] = test::graph::Add(g, in0, in1);
  }
  // The final output node "out".
  test::graph::Send(g, nodes.back(), "b", BOB, 1, ALICE);
}

TEST_F(ExecutorTest, RandomTree) {
  std::unique_ptr<Graph> g(new Graph(OpRegistry::Global()));
  BuildTree(4096, g.get());
  Create(std::move(g));
  Rendezvous::Args args;
  TF_ASSERT_OK(
      rendez_->Send(Key(ALICE, kIncarnation, BOB, "a"), args, V(1.0), false));
  TF_ASSERT_OK(Run(rendez_));
  Tensor out = V(-1);
  bool is_dead = false;
  TF_ASSERT_OK(
      rendez_->Recv(Key(BOB, kIncarnation, ALICE, "b"), args, &out, &is_dead));
  EXPECT_EQ(4096.0, V(out));
}

void BuildConcurrentAddAssign(Graph* g) {
  auto one = test::graph::Constant(g, V(1.0));
  // A variable holds one float.
  auto var = test::graph::Var(g, DT_FLOAT, TensorShape({}));
  // Initilize the variable with 1.0.
  auto init = test::graph::Assign(g, var, one);
  // Output
  auto out = test::graph::Send(g, var, "out", ALICE, kIncarnation, BOB);
  // Have many concurrent computation. Each does v = v + 1.
  for (int i = 0; i < 1024; ++i) {
    auto add = test::graph::Add(g, var, one);
    g->AddControlEdge(init, add);  // Ensures run after init.
    auto assign = test::graph::Assign(g, var, add);
    g->AddControlEdge(assign, out);
  }
}

#ifndef THREAD_SANITIZER
TEST_F(ExecutorTest, ConcurrentAddAssign) {
  std::unique_ptr<Graph> g(new Graph(OpRegistry::Global()));
  BuildConcurrentAddAssign(g.get());
  Create(std::move(g));
  for (int iters = 0; iters < 16; ++iters) {
    Rendezvous* rendez = NewLocalRendezvous();
    TF_ASSERT_OK(Run(rendez));
    Rendezvous::Args args;
    Tensor out;
    bool is_dead;
    TF_ASSERT_OK(rendez->Recv(Key(ALICE, kIncarnation, BOB, "out"), args, &out,
                              &is_dead));
    VLOG(1) << "Get " << V(out);
    EXPECT_LE(V(out), 1025.0);
    rendez->Unref();
  }
}
#endif

TEST_F(ExecutorTest, SimpleSwitchLive) {
  std::unique_ptr<Graph> g(new Graph(OpRegistry::Global()));
  auto in0 = test::graph::Recv(g.get(), "a", "float", ALICE, 1, BOB);
  auto in1 = test::graph::Constant(g.get(), VB(false));
  auto tmp = test::graph::Switch(g.get(), in0, in1);
  test::graph::Send(g.get(), tmp, "c", BOB, 1, ALICE);
  Create(std::move(g));
  Rendezvous::Args args;
  TF_ASSERT_OK(rendez_->Send(Key(ALICE, kIncarnation, BOB, "a"), args, V(1.0),
                             false));  // in0 = 1.0
  TF_ASSERT_OK(Run(rendez_));
  Tensor out = V(-1);
  bool is_dead = false;
  TF_ASSERT_OK(
      rendez_->Recv(Key(BOB, kIncarnation, ALICE, "c"), args, &out, &is_dead));
  EXPECT_EQ(1.0, V(out));  // out = 1.0
  EXPECT_FALSE(is_dead);
}

TEST_F(ExecutorTest, SimpleSwitchDead) {
  std::unique_ptr<Graph> g(new Graph(OpRegistry::Global()));
  auto in0 = test::graph::Recv(g.get(), "a", "float", ALICE, 1, BOB);
  auto in1 = test::graph::Constant(g.get(), VB(true));
  auto tmp = test::graph::Switch(g.get(), in0, in1);
  test::graph::Send(g.get(), tmp, "c", BOB, 1, ALICE);
  Create(std::move(g));
  Rendezvous::Args args;
  TF_ASSERT_OK(rendez_->Send(Key(ALICE, kIncarnation, BOB, "a"), args, V(1.0),
                             false));  // in0 = 1.0
  TF_ASSERT_OK(Run(rendez_));
  Tensor out = V(-1);
  bool is_dead = false;
  TF_ASSERT_OK(
      rendez_->Recv(Key(BOB, kIncarnation, ALICE, "c"), args, &out, &is_dead));
  EXPECT_TRUE(is_dead);
}

TEST_F(ExecutorTest, Abort) {
  // e = a + b + c + d
  std::unique_ptr<Graph> g(new Graph(OpRegistry::Global()));
  auto in0 = test::graph::Recv(g.get(), "a", "float", ALICE, 1, BOB);
  auto in1 = test::graph::Recv(g.get(), "b", "float", ALICE, 1, BOB);
  auto in2 = test::graph::Recv(g.get(), "c", "float", ALICE, 1, BOB);
  auto in3 = test::graph::Recv(g.get(), "d", "float", ALICE, 1, BOB);
  auto add0 = test::graph::Add(g.get(), in0, in1);
  auto add1 = test::graph::Add(g.get(), in2, in3);
  auto add2 = test::graph::Add(g.get(), add0, add1);
  test::graph::Send(g.get(), add2, "e", BOB, 1, ALICE);
  Create(std::move(g));

  // Needs 4 inputs (recv). One of them is aborted.
  rendez_->Ref();
  SchedClosure([this]() {
    Env::Default()->SleepForMicroseconds(100 * 1000);
    Status s = rendez_->Send(Key(ALICE, kIncarnation, BOB, "a"),
                             Rendezvous::Args(), V(1.0), false);
    rendez_->Unref();
  });
  rendez_->Ref();
  SchedClosure([this]() {
    Env::Default()->SleepForMicroseconds(100 * 1000);
    Status s = rendez_->Send(Key(ALICE, kIncarnation, BOB, "b"),
                             Rendezvous::Args(), V(1.0), false);
    rendez_->Unref();
  });
  rendez_->Ref();
  SchedClosure([this]() {
    Env::Default()->SleepForMicroseconds(100 * 1000);
    Status s = rendez_->Send(Key(ALICE, kIncarnation, BOB, "c"),
                             Rendezvous::Args(), V(1.0), false);
    rendez_->Unref();
  });
  rendez_->Ref();
  SchedClosure([this]() {
    Env::Default()->SleepForMicroseconds(100 * 1000);
    rendez_->StartAbort(errors::Aborted(""));
    rendez_->Unref();
  });
  EXPECT_TRUE(errors::IsAborted(Run(rendez_)));
  Tensor out = V(-1);
  bool is_dead = false;
  EXPECT_TRUE(errors::IsAborted(rendez_->Recv(
      Key(BOB, kIncarnation, ALICE, "c"), Rendezvous::Args(), &out, &is_dead)));
  // At this point there can still be pending (albeit Aborted) Send
  // closures holding Refs on rendez_.  We need to wait for them, or
  // else there can be a memory leak at termination.
  while (!rendez_->RefCountIsOne())
    ;
}

TEST_F(ExecutorTest, RecvInvalidDtype) {
  std::unique_ptr<Graph> g(new Graph(OpRegistry::Global()));
  // An input vector of type float of size 1.
  auto one = test::graph::Recv(g.get(), "one", "float", ALICE, 1, BOB);
  // A floating point variable vector of size 1.
  auto var = test::graph::Var(g.get(), DT_FLOAT, TensorShape({1}));
  // Initialize the variable with input.
  auto init = test::graph::Assign(g.get(), var, one);
  // Output
  auto* two = test::graph::Send(g.get(), var, "two", BOB, 1, ALICE);
  g->AddControlEdge(init, two);  // Ensures run after init.
  Create(std::move(g));
  Rendezvous* rendez = NewLocalRendezvous();
  // Send a double instead of float.
  TF_ASSERT_OK(rendez->Send(Key(ALICE, 1, BOB, "one"), Rendezvous::Args(),
                            VD(1.0), false));
  // Fails due to invalid dtype.
  EXPECT_TRUE(errors::IsInternal(Run(rendez)));
  Tensor output;
  bool is_dead;
  EXPECT_TRUE(errors::IsInternal(rendez->Recv(
      Key(BOB, 1, ALICE, "two"), Rendezvous::Args(), &output, &is_dead)));
  rendez->Unref();
}

TEST_F(ExecutorTest, RecvInvalidRefDtype) {
  std::unique_ptr<Graph> g(new Graph(OpRegistry::Global()));
  // A var that always produces as invalid dtype.
  auto var = test::graph::InvalidRefType(g.get(), DT_FLOAT, DT_DOUBLE);
  test::graph::Send(g.get(), var, "out", BOB, 1, ALICE);
  Create(std::move(g));
  Rendezvous* rendez = NewLocalRendezvous();
  EXPECT_TRUE(errors::IsInternal(Run(rendez)));
  Tensor output;
  bool is_dead;
  EXPECT_TRUE(errors::IsInternal(rendez->Recv(
      Key(BOB, 1, ALICE, "out"), Rendezvous::Args(), &output, &is_dead)));
  rendez->Unref();
}

// Create a graph that is 'depth' deep. At each level, fan-in and fan-out a
// maximum of 'width' nodes. All nodes are no-ops and all dependencies are
// control dependencies.
static void BM_executor(int iters, int width, int depth) {
#ifdef PLATFORM_GOOGLE
  BenchmarkUseRealTime();
#endif  // PLATFORM_GOOGLE
  Graph* g = new Graph(OpRegistry::Global());
  random::PhiloxRandom philox(1729, 17);
  random::SimplePhilox rand(&philox);
  uint64 cur = 0;
  uint32 r = 1 + rand.Rand32() % width;
  std::vector<Node*> ready_nodes;
  for (int i = 0; i < r; ++i) {
    ready_nodes.push_back(test::graph::NoOp(g, {}));
    ++cur;
  }
  for (int i = 0; i < depth; ++i) {
    std::random_shuffle(ready_nodes.begin(), ready_nodes.end());
    r = 1 + rand.Rand32() % (ready_nodes.size());
    std::vector<Node*> control_inputs;
    for (int j = 0; j < r; ++j) {
      control_inputs.push_back(ready_nodes.back());
      ready_nodes.pop_back();
    }
    Node* n = test::graph::NoOp(g, control_inputs);
    ++cur;
    r = 1 + rand.Rand32() % width;
    for (int j = 0; j < r; ++j) {
      ready_nodes.push_back(test::graph::NoOp(g, {n}));
      ++cur;
    }
  }
#ifdef PLATFORM_GOOGLE
  SetBenchmarkLabel(strings::StrCat("Nodes = ", cur));
  SetBenchmarkItemsProcessed(cur * static_cast<int64>(iters));
#endif  // PLATFORM_GOOGLE
  test::Benchmark("cpu", g).Run(iters);
}

// Tall skinny graphs
BENCHMARK(BM_executor)->ArgPair(16, 1024);
BENCHMARK(BM_executor)->ArgPair(32, 8192);

// Short fat graphs
BENCHMARK(BM_executor)->ArgPair(1024, 16);
BENCHMARK(BM_executor)->ArgPair(8192, 32);

// Tall fat graph
BENCHMARK(BM_executor)->ArgPair(1024, 1024);

static void BM_FeedInputFetchOutput(int iters) {
  Graph* g = new Graph(OpRegistry::Global());
  // z = x + y: x and y are provided as benchmark inputs.  z is the
  // output of the benchmark.  Conceptually, the caller is ALICE, the
  // benchmark is BOB.
  Node* x = test::graph::Recv(g, "x", "float", ALICE, 1, BOB);
  Node* y = test::graph::Recv(g, "y", "float", ALICE, 1, BOB);
  Node* sum = test::graph::Add(g, x, y);
  Node* z = test::graph::Send(g, sum, "z", BOB, 1, ALICE);
  Tensor val(DT_FLOAT, TensorShape({}));
  val.scalar<float>()() = 3.14;
#ifdef PLATFORM_GOOGLE
  SetBenchmarkItemsProcessed(static_cast<int64>(iters));
#endif  // PLATFORM_GOOGLE
  test::Benchmark("cpu", g).RunWithArgs({{x, val}, {y, val}}, {z}, iters);
}
BENCHMARK(BM_FeedInputFetchOutput);

}  // namespace tensorflow
