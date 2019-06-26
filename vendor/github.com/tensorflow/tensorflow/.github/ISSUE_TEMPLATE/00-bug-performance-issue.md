---
name: Bug/Performance Issue
about: Use this template for reporting a bug or a performance issue.

---

<em>Please make sure that this is a bug. As per our [GitHub Policy](https://github.com/tensorflow/tensorflow/blob/master/ISSUES.md), we only address code/doc bugs, performance issues, feature requests and build/installation issues on GitHub. tag:bug_template</em>

**System information**
- Have I written custom code (as opposed to using a stock example script provided in TensorFlow):
- OS Platform and Distribution (e.g., Linux Ubuntu 16.04):
- Mobile device (e.g. iPhone 8, Pixel 2, Samsung Galaxy) if the issue happens on mobile device:
- TensorFlow installed from (source or binary):
- TensorFlow version (use command below):
- Python version:
- Bazel version (if compiling from source):
- GCC/Compiler version (if compiling from source):
- CUDA/cuDNN version:
- GPU model and memory:


You can collect some of this information using our environment capture [script](https://github.com/tensorflow/tensorflow/tree/master/tools/tf_env_collect.sh)
You can also obtain the TensorFlow version with
python -c "import tensorflow as tf; print(tf.GIT_VERSION, tf.VERSION)"

**Describe the current behavior**

**Describe the expected behavior**

**Code to reproduce the issue**
Provide a reproducible test case that is the bare minimum necessary to generate the problem.

**Other info / logs**
Include any logs or source code that would be helpful to diagnose the problem. If including tracebacks, please include the full traceback. Large logs and files should be attached.
