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

package org.tensorflow;

/**
 * A Graph node that performs computation on Tensors.
 *
 * <p>An Operation is a node in a {@link Graph} that takes zero or more {@link Tensor}s (produced by
 * other Operations in the Graph) as input, and produces zero or more {@link Tensor}s as output.
 *
 * <p>Operation instances are valid only as long as the Graph they are a part of is valid. Thus, if
 * {@link Graph#close()} has been invoked, then methods on the Operation instance may fail with an
 * {@code IllegalStateException}.
 *
 * <p>Operation instances are immutable and thread-safe.
 */
public final class Operation {

  // Create an Operation instance referring to an operation in g, with the given handle to the C
  // TF_Operation object.  The handle is valid only as long as g has not been closed, hence it is
  // called unsafeHandle.  Graph.ref() is used to safely use the unsafeHandle.
  Operation(Graph g, long unsafeNativeHandle) {
    this.graph = g;
    this.unsafeNativeHandle = unsafeNativeHandle;
  }

  /** Returns the full name of the Operation. */
  public String name() {
    Graph.Reference r = graph.ref();
    try {
      return name(unsafeNativeHandle);
    } finally {
      r.close();
    }
  }

  /**
   * Returns the type of the operation, i.e., the name of the computation performed by the
   * operation.
   */
  public String type() {
    Graph.Reference r = graph.ref();
    try {
      return type(unsafeNativeHandle);
    } finally {
      r.close();
    }
  }

  /** Returns the number of tensors produced by this operation. */
  public int numOutputs() {
    Graph.Reference r = graph.ref();
    try {
      return numOutputs(unsafeNativeHandle);
    } finally {
      r.close();
    }
  }

  /**
   * Returns the size of the list of Tensors produced by this operation.
   *
   * <p>An Operation has multiple named outputs, each of which produces either a single tensor or a
   * list of tensors. This method returns the size of the list of tensors for a specific named
   * output of the operation.
   *
   * @param name identifier of the list of tensors (of which there may be many) produced by this
   *     operation.
   * @return the size of the list of Tensors produced by this named output.
   * @throws IllegalArgumentException if this operation has no output with the provided name.
   */
  public int outputListLength(final String name) {
    Graph.Reference r = graph.ref();
    try {
      return outputListLength(unsafeNativeHandle, name);
    } finally {
      r.close();
    }
  }

  /**
   * Returns symbolic handles to a list of tensors produced by this operation.
   *
   * @param idx index of the first tensor of the list
   * @param length number of tensors in the list
   * @return array of {@code Output}
   */
  public Output<?>[] outputList(int idx, int length) {
    Output<?>[] outputs = new Output<?>[length];
    for (int i = 0; i < length; ++i) {
      outputs[i] = output(idx + i);
    }
    return outputs;
  }

  /**
   * Returns a symbolic handle to one of the tensors produced by this operation.
   *
   * <p>Warning: Does not check that the type of the tensor matches T. It is recommended to call
   * this method with an explicit type parameter rather than letting it be inferred, e.g. {@code
   * operation.<Integer>output(0)}
   *
   * @param <T> The expected element type of the tensors produced by this output.
   * @param idx The index of the output among the outputs produced by this operation.
   */
  @SuppressWarnings({"rawtypes", "unchecked"})
  public <T> Output<T> output(int idx) {
    return new Output(this, idx);
  }

  @Override
  public int hashCode() {
    return Long.valueOf(unsafeNativeHandle).hashCode();
  }

  @Override
  public boolean equals(Object o) {
    if (o == this) {
      return true;
    }
    if (!(o instanceof Operation)) {
      return false;
    }
    Operation that = (Operation) o;
    if (graph != that.graph) {
      return false;
    }

    // The graph object is known to be identical here, so this one
    // reference is sufficient to validate the use of native pointers
    // in both objects.
    Graph.Reference r = graph.ref();
    try {
      return unsafeNativeHandle == that.unsafeNativeHandle;
    } finally {
      r.close();
    }
  }

  @Override
  public String toString() {
    return String.format("<%s '%s'>", type(), name());
  }

  /**
   * Returns the size of the given inputs list of Tensors for this operation.
   *
   * <p>An Operation has multiple named inputs, each of which contains either a single tensor or a
   * list of tensors. This method returns the size of the list of tensors for a specific named input
   * of the operation.
   *
   * @param name identifier of the list of tensors (of which there may be many) inputs to this
   *     operation.
   * @return the size of the list of Tensors produced by this named input.
   * @throws IllegalArgumentException if this operation has no input with the provided name.
   */
  public int inputListLength(final String name) {
    Graph.Reference r = graph.ref();
    try {
      return inputListLength(unsafeNativeHandle, name);
    } finally {
      r.close();
    }
  }

  long getUnsafeNativeHandle() {
    return unsafeNativeHandle;
  }

  // Package private, meant primarily for the public Output.shape() method.
  long[] shape(int output) {
    Graph.Reference r = graph.ref();
    try {
      return shape(r.nativeHandle(), unsafeNativeHandle, output);
    } finally {
      r.close();
    }
  }

  // Package private, meant primarily for the public Output.dataType() method.
  DataType dtype(int output) {
    Graph.Reference r = graph.ref();
    try {
      return DataType.fromC(dtype(r.nativeHandle(), unsafeNativeHandle, output));
    } finally {
      r.close();
    }
  }

  private final long unsafeNativeHandle;

  private final Graph graph;

  private static native String name(long handle);

  private static native String type(long handle);

  private static native int numOutputs(long handle);

  private static native int outputListLength(long handle, String name);

  private static native int inputListLength(long handle, String name);

  private static native long[] shape(long graphHandle, long opHandle, int output);

  private static native int dtype(long graphHandle, long opHandle, int output);
}
