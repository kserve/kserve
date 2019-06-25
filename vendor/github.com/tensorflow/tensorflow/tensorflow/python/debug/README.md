# TensorFlow Debugger (TFDBG)

[TOC]

TensorFlow Debugger (TFDBG) is a specialized debugger for TensorFlow's computation
graphs. It provides access to internal graph structures and tensor values at
TensorFlow runtime.

<!-- TODO(cais): Add release notes starting from 1.3. -->

## Why TFDBG?

In TensorFlow's current
[computation-graph framework](https://www.tensorflow.org/get_started/get_started#the_computational_graph),
almost all actual computation after graph construction happens in a single
Python function, namely
[tf.Session.run](https://www.tensorflow.org/api_docs/python/tf/Session#run).
Basic Python debugging tools such as [pdb](https://docs.python.org/2/library/pdb.html)
cannot be used to debug `Session.run`, due to the fact that TensorFlow's graph
execution happens in the underlying C++ layer. C++ debugging tools such as
[gdb](https://www.gnu.org/software/gdb/) are not ideal either, because of their
inability to recognize and organize the stack frames and variables in a way
relevant to TensorFlow's operations, tensors and other graph constructs.

TFDBG addresses these limitations. Among the features provided by TFDBG, the
following ones are designed to facilitate runtime debugging of TensorFlow
models:

* Easy access through session wrappers
* Easy integration with common high-level APIs, such as
  [TensorFlow Estimators](https://www.tensorflow.org/guide/estimators) and
  [Keras](https://keras.io/)
* Inspection of runtime tensor values and node connections
* Conditional breaking after runs that generate tensors satisfying given
  predicates, which makes common debugging tasks such as tracing the origin
  of infinities and [NaNs](https://en.wikipedia.org/wiki/NaN) easier
* Association of nodes and tensors in graphs with Python source lines
* Profiling of models at the level of graph nodes and Python source lines.
(Omitted internal-only feature)
* A [gRPC](https://grpc.io/)-based remote debugging protocol, which allows us to
  build a browser-based graphical user interface (GUI) for TFDBG: the
  [TensorBoard Debugger Plugin](https://github.com/tensorflow/tensorboard/blob/master/tensorboard/plugins/debugger/README.md).

## How to use TFDBG?

* For a walkthrough of TFDBG command-line interface, see https://www.tensorflow.org/guide/debugger.
* For information on the web GUI of TFDBG (TensorBoard Debugger Plugin), see
  [this README](https://github.com/tensorflow/tensorboard/blob/master/tensorboard/plugins/debugger/README.md).
* For programmatic use of the API of TFDBG, see https://www.tensorflow.org/api_docs/python/tfdbg.


## Related Publications

* Cai, S., Breck E., Nielsen E., Salib M., Sculley D. (2016) TensorFlow Debugger:
  Debugging Dataflow Graphs for Machine Learning. https://research.google.com/pubs/pub45789.html
