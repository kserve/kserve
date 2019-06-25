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

package org.tensorflow.lite;

import java.io.File;
import java.nio.ByteBuffer;
import java.nio.MappedByteBuffer;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import org.checkerframework.checker.nullness.qual.NonNull;

/**
 * Driver class to drive model inference with TensorFlow Lite.
 *
 * <p>A {@code Interpreter} encapsulates a pre-trained TensorFlow Lite model, in which operations
 * are executed for model inference.
 *
 * <p>For example, if a model takes only one input and returns only one output:
 *
 * <pre>{@code
 * try (Interpreter interpreter = new Interpreter(file_of_a_tensorflowlite_model)) {
 *   interpreter.run(input, output);
 * }
 * }</pre>
 *
 * <p>If a model takes multiple inputs or outputs:
 *
 * <pre>{@code
 * Object[] inputs = {input0, input1, ...};
 * Map<Integer, Object> map_of_indices_to_outputs = new HashMap<>();
 * ByteBuffer ith_output = ByteBuffer.allocateDirect(3 * 2 * 4 * 4);  // Float tensor, shape 3x2x4.
 * ith_output.order(ByteOrder.nativeOrder());
 * map_of_indices_to_outputs.put(i, ith_output);
 * try (Interpreter interpreter = new Interpreter(file_of_a_tensorflowlite_model)) {
 *   interpreter.runForMultipleInputsOutputs(inputs, map_of_indices_to_outputs);
 * }
 * }</pre>
 *
 * <p>If a model takes or produces string tensors:
 *
 * <pre>{@code
 * String[] input = {"foo", "bar"};  // Input tensor shape is [2].
 * String[] output = new String[3][2];  // Output tensor shape is [3, 2].
 * try (Interpreter interpreter = new Interpreter(file_of_a_tensorflowlite_model)) {
 *   interpreter.runForMultipleInputsOutputs(input, output);
 * }
 * }</pre>
 *
 * <p>Orders of inputs and outputs are determined when converting TensorFlow model to TensorFlowLite
 * model with Toco, as are the default shapes of the inputs.
 *
 * <p>When inputs are provided as (multi-dimensional) arrays, the corresponding input tensor(s) will
 * be implicitly resized according to that array's shape. When inputs are provided as {@link
 * ByteBuffer} types, no implicit resizing is done; the caller must ensure that the {@link
 * ByteBuffer} byte size either matches that of the corresponding tensor, or that they first resize
 * the tensor via {@link #resizeInput()}. Tensor shape and type information can be obtained via the
 * {@link Tensor} class, available via {@link #getInputTensor(int)} and {@link
 * #getOutputTensor(int)}.
 *
 * <p><b>WARNING:</b>Instances of a {@code Interpreter} is <b>not</b> thread-safe. A {@code
 * Interpreter} owns resources that <b>must</b> be explicitly freed by invoking {@link #close()}
 */
public final class Interpreter implements AutoCloseable {

  /** An options class for controlling runtime interpreter behavior. */
  public static class Options {
    public Options() {}

    /**
     * Sets the number of threads to be used for ops that support multi-threading. Defaults to a
     * platform-dependent value.
     */
    public Options setNumThreads(int numThreads) {
      this.numThreads = numThreads;
      return this;
    }

    /** Sets whether to use NN API (if available) for op execution. Defaults to false (disabled). */
    public Options setUseNNAPI(boolean useNNAPI) {
      this.useNNAPI = useNNAPI;
      return this;
    }

    /**
     * Sets whether to allow float16 precision for FP32 calculation when possible. Defaults to false
     * (disallow).
     * WARNING: This is an experimental API and subject to change.
     */
    public Options setAllowFp16PrecisionForFp32(boolean allow) {
      this.allowFp16PrecisionForFp32 = allow;
      return this;
    }

    /**
     * Adds a {@link Delegate} to be applied during interpreter creation.
     *
     * <p>WARNING: This is an experimental interface that is subject to change.
     */
    public Options addDelegate(Delegate delegate) {
      delegates.add(delegate);
      return this;
    }

    int numThreads = -1;
    boolean useNNAPI = false;
    boolean allowFp16PrecisionForFp32 = false;
    final List<Delegate> delegates = new ArrayList<>();
  }

  /**
   * Initializes a {@code Interpreter}
   *
   * @param modelFile: a File of a pre-trained TF Lite model.
   */
  public Interpreter(@NonNull File modelFile) {
    this(modelFile, /*options = */ null);
  }

  /**
   * Initializes a {@code Interpreter} and specifies the number of threads used for inference.
   *
   * @param modelFile: a file of a pre-trained TF Lite model
   * @param numThreads: number of threads to use for inference
   * @deprecated Prefer using the {@link #Interpreter(File,Options)} constructor. This method will
   *     be removed in a future release.
   */
  @Deprecated
  public Interpreter(@NonNull File modelFile, int numThreads) {
    this(modelFile, new Options().setNumThreads(numThreads));
  }

  /**
   * Initializes a {@code Interpreter} and specifies the number of threads used for inference.
   *
   * @param modelFile: a file of a pre-trained TF Lite model
   * @param options: a set of options for customizing interpreter behavior
   */
  public Interpreter(@NonNull File modelFile, Options options) {
    wrapper = new NativeInterpreterWrapper(modelFile.getAbsolutePath(), options);
  }

  /**
   * Initializes a {@code Interpreter} with a {@code ByteBuffer} of a model file.
   *
   * <p>The ByteBuffer should not be modified after the construction of a {@code Interpreter}. The
   * {@code ByteBuffer} can be either a {@code MappedByteBuffer} that memory-maps a model file, or a
   * direct {@code ByteBuffer} of nativeOrder() that contains the bytes content of a model.
   */
  public Interpreter(@NonNull ByteBuffer byteBuffer) {
    this(byteBuffer, /* options= */ null);
  }

  /**
   * Initializes a {@code Interpreter} with a {@code ByteBuffer} of a model file and specifies the
   * number of threads used for inference.
   *
   * <p>The ByteBuffer should not be modified after the construction of a {@code Interpreter}. The
   * {@code ByteBuffer} can be either a {@code MappedByteBuffer} that memory-maps a model file, or a
   * direct {@code ByteBuffer} of nativeOrder() that contains the bytes content of a model.
   *
   * @deprecated Prefer using the {@link #Interpreter(ByteBuffer,Options)} constructor. This method
   *     will be removed in a future release.
   */
  @Deprecated
  public Interpreter(@NonNull ByteBuffer byteBuffer, int numThreads) {
    this(byteBuffer, new Options().setNumThreads(numThreads));
  }

  /**
   * Initializes a {@code Interpreter} with a {@code MappedByteBuffer} to the model file.
   *
   * <p>The {@code MappedByteBuffer} should remain unchanged after the construction of a {@code
   * Interpreter}.
   *
   * @deprecated Prefer using the {@link #Interpreter(ByteBuffer,Options)} constructor. This method
   *     will be removed in a future release.
   */
  @Deprecated
  public Interpreter(@NonNull MappedByteBuffer mappedByteBuffer) {
    this(mappedByteBuffer, /* options= */ null);
  }

  /**
   * Initializes a {@code Interpreter} with a {@code ByteBuffer} of a model file and a set of custom
   * {@link #Options}.
   *
   * <p>The ByteBuffer should not be modified after the construction of a {@code Interpreter}. The
   * {@code ByteBuffer} can be either a {@code MappedByteBuffer} that memory-maps a model file, or a
   * direct {@code ByteBuffer} of nativeOrder() that contains the bytes content of a model.
   */
  public Interpreter(@NonNull ByteBuffer byteBuffer, Options options) {
    wrapper = new NativeInterpreterWrapper(byteBuffer, options);
  }

  /**
   * Runs model inference if the model takes only one input, and provides only one output.
   *
   * <p>Warning: The API runs much faster if {@link ByteBuffer} is used as input data type. Please
   * consider using {@link ByteBuffer} to feed primitive input data for better performance.
   *
   * @param input an array or multidimensional array, or a {@link ByteBuffer} of primitive types
   *     including int, float, long, and byte. {@link ByteBuffer} is the preferred way to pass large
   *     input data for primitive types, whereas string types require using the (multi-dimensional)
   *     array input path. When {@link ByteBuffer} is used, its content should remain unchanged
   *     until model inference is done.
   * @param output a multidimensional array of output data, or a {@link ByteBuffer} of primitive
   *     types including int, float, long, and byte.
   */
  public void run(@NonNull Object input, @NonNull Object output) {
    Object[] inputs = {input};
    Map<Integer, Object> outputs = new HashMap<>();
    outputs.put(0, output);
    runForMultipleInputsOutputs(inputs, outputs);
  }

  /**
   * Runs model inference if the model takes multiple inputs, or returns multiple outputs.
   *
   * <p>Warning: The API runs much faster if {@link ByteBuffer} is used as input data type. Please
   * consider using {@link ByteBuffer} to feed primitive input data for better performance.
   *
   * @param inputs an array of input data. The inputs should be in the same order as inputs of the
   *     model. Each input can be an array or multidimensional array, or a {@link ByteBuffer} of
   *     primitive types including int, float, long, and byte. {@link ByteBuffer} is the preferred
   *     way to pass large input data, whereas string types require using the (multi-dimensional)
   *     array input path. When {@link ByteBuffer} is used, its content should remain unchanged
   *     until model inference is done.
   * @param outputs a map mapping output indices to multidimensional arrays of output data or {@link
   *     ByteBuffer}s of primitive types including int, float, long, and byte. It only needs to keep
   *     entries for the outputs to be used.
   */
  public void runForMultipleInputsOutputs(
      @NonNull Object[] inputs, @NonNull Map<Integer, Object> outputs) {
    checkNotClosed();
    wrapper.run(inputs, outputs);
  }

  /**
   * Resizes idx-th input of the native model to the given dims.
   *
   * <p>IllegalArgumentException will be thrown if it fails to resize.
   */
  public void resizeInput(int idx, @NonNull int[] dims) {
    checkNotClosed();
    wrapper.resizeInput(idx, dims);
  }

  /** Gets the number of input tensors. */
  public int getInputTensorCount() {
    checkNotClosed();
    return wrapper.getInputTensorCount();
  }

  /**
   * Gets index of an input given the op name of the input.
   *
   * <p>IllegalArgumentException will be thrown if the op name does not exist in the model file used
   * to initialize the {@link Interpreter}.
   */
  public int getInputIndex(String opName) {
    checkNotClosed();
    return wrapper.getInputIndex(opName);
  }

  /**
   * Gets the Tensor associated with the provdied input index.
   *
   * <p>IllegalArgumentException will be thrown if the provided index is invalid.
   */
  public Tensor getInputTensor(int inputIndex) {
    checkNotClosed();
    return wrapper.getInputTensor(inputIndex);
  }

  /** Gets the number of output Tensors. */
  public int getOutputTensorCount() {
    checkNotClosed();
    return wrapper.getOutputTensorCount();
  }

  /**
   * Gets index of an output given the op name of the output.
   *
   * <p>IllegalArgumentException will be thrown if the op name does not exist in the model file used
   * to initialize the {@link Interpreter}.
   */
  public int getOutputIndex(String opName) {
    checkNotClosed();
    return wrapper.getOutputIndex(opName);
  }

  /**
   * Gets the Tensor associated with the provdied output index.
   *
   * <p>IllegalArgumentException will be thrown if the provided index is invalid.
   */
  public Tensor getOutputTensor(int outputIndex) {
    checkNotClosed();
    return wrapper.getOutputTensor(outputIndex);
  }

  /**
   * Returns native inference timing.
   *
   * <p>IllegalArgumentException will be thrown if the model is not initialized by the {@link
   * Interpreter}.
   */
  public Long getLastNativeInferenceDurationNanoseconds() {
    checkNotClosed();
    return wrapper.getLastNativeInferenceDurationNanoseconds();
  }

  /**
   * Turns on/off Android NNAPI for hardware acceleration when it is available.
   *
   * @deprecated Prefer using {@link Options#setUseNNAPI(boolean)} directly for enabling NN API.
   *     This method will be removed in a future release.
   */
  @Deprecated
  public void setUseNNAPI(boolean useNNAPI) {
    checkNotClosed();
    wrapper.setUseNNAPI(useNNAPI);
  }

  /**
   * Sets the number of threads to be used for ops that support multi-threading.
   *
   * @deprecated Prefer using {@link Options#setNumThreads(int)} directly for controlling thread
   *     multi-threading. This method will be removed in a future release.
   */
  @Deprecated
  public void setNumThreads(int numThreads) {
    checkNotClosed();
    wrapper.setNumThreads(numThreads);
  }

  /** Release resources associated with the {@code Interpreter}. */
  @Override
  public void close() {
    if (wrapper != null) {
      wrapper.close();
      wrapper = null;
    }
  }

  @Override
  protected void finalize() throws Throwable {
    try {
      close();
    } finally {
      super.finalize();
    }
  }

  private void checkNotClosed() {
    if (wrapper == null) {
      throw new IllegalStateException("Internal error: The Interpreter has already been closed.");
    }
  }

  NativeInterpreterWrapper wrapper;
}
