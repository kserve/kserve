# TensorFlow

TensorFlow is a computational dataflow graph library.

## Getting started


### Python API example
The following is an example python code to do a simple matrix multiply
of two constants and get the result from a locally-running TensorFlow
process.

First, bring in tensorflow python dependency

//third_party/py/tensorflow

to get the python TensorFlow API.

Then:

```python
import tensorflow as tf

with tf.Session():
  input1 = tf.constant(1.0, shape=[1, 1], name="input1")
  input2 = tf.constant(2.0, shape=[1, 1], name="input2")
  output = tf.matmul(input1, input2)

  # Run graph and fetch the output
  result = output.eval()
  print result
```

### C++ API Example

If you are running TensorFlow locally, link your binary with

//third_party/tensorflow/core

and link in the operation implementations you want to supported, e.g.,

//third_party/tensorflow/core:kernels

An example program to take a GraphDef and run it using TensorFlow
using the C++ Session API:

```c++
#include <memory>
#include <string>
#include <vector>

#include "tensorflow/core/framework/graph.pb.h"
#include "tensorflow/core/public/session.h"
#include "tensorflow/core/framework/tensor.h"

int main(int argc, char** argv) {
  // Construct your graph.
  tensorflow::GraphDef graph = ...;

  // Create a Session running TensorFlow locally in process.
  std::unique_ptr<tensorflow::Session> session(tensorflow::NewSession({}));

  // Initialize the session with the graph.
  tensorflow::Status s = session->Create(graph);
  if (!s.ok()) { ... }

  // Specify the 'feeds' of your network if needed.
  std::vector<std::pair<string, tensorflow::Tensor>> inputs;

  // Run the session, asking for the first output of "my_output".
  std::vector<tensorflow::Tensor> outputs;
  s = session->Run(inputs, {"my_output:0"}, {}, &outputs);
  if (!s.ok()) { ... }

  // Do something with your outputs
  auto output_vector = outputs[0].vec<float>();
  if (output_vector(0) > 0.5) { ... }

  // Close the session.
  session->Close();

  return 0;
}
```

For a more fully-featured C++ example, see
`tensorflow/cc/tutorials/example_trainer.cc`
