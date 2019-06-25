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
 * SavedModelBundle represents a model loaded from storage.
 *
 * <p>The model consists of a description of the computation (a {@link Graph}), a {@link Session}
 * with tensors (e.g., parameters or variables in the graph) initialized to values saved in storage,
 * and a description of the model (a serialized representation of a <a
 * href="https://www.tensorflow.org/code/tensorflow/core/protobuf/meta_graph.proto">MetaGraphDef
 * protocol buffer</a>).
 */
public class SavedModelBundle implements AutoCloseable {
  /** Options for loading a SavedModel. */
  public static final class Loader {
    /** Load a <code>SavedModelBundle</code> with the configured options. */
    public SavedModelBundle load() {
      return SavedModelBundle.load(exportDir, tags, configProto, runOptions);
    }

    /**
     * Sets options to use when executing model initialization operations.
     *
     * @param options Serialized <a
     *     href="https://www.tensorflow.org/code/tensorflow/core/protobuf/config.proto">RunOptions
     *     protocol buffer</a>.
     */
    public Loader withRunOptions(byte[] options) {
      this.runOptions = options;
      return this;
    }

    /**
     * Set configuration of the <code>Session</code> object created when loading the model.
     *
     * @param configProto Serialized <a
     *     href="https://www.tensorflow.org/code/tensorflow/core/protobuf/config.proto">ConfigProto
     *     protocol buffer</a>.
     */
    public Loader withConfigProto(byte[] configProto) {
      this.configProto = configProto;
      return this;
    }

    /**
     * Sets the set of tags that identify the specific graph in the saved model to load.
     *
     * @param tags the tags identifying the specific MetaGraphDef to load.
     */
    public Loader withTags(String... tags) {
      this.tags = tags;
      return this;
    }

    private Loader(String exportDir) {
      this.exportDir = exportDir;
    }

    private String exportDir = null;
    private String[] tags = null;
    private byte[] configProto = null;
    private byte[] runOptions = null;
  }

  /**
   * Load a saved model from an export directory. The model that is being loaded should be created
   * using the <a href="https://www.tensorflow.org/api_docs/python/tf/saved_model">Saved Model
   * API</a>.
   *
   * <p>This method is a shorthand for:
   *
   * <pre>{@code
   * SavedModelBundler.loader().withTags(tags).load();
   * }</pre>
   *
   * @param exportDir the directory path containing a saved model.
   * @param tags the tags identifying the specific metagraphdef to load.
   * @return a bundle containing the graph and associated session.
   */
  public static SavedModelBundle load(String exportDir, String... tags) {
    return loader(exportDir).withTags(tags).load();
  }

  /**
   * Load a saved model.
   *
   * <p/>Returns a <code>Loader</code> object that can set configuration options before actually
   * loading the model,
   *
   * @param exportDir the directory path containing a saved model.
   */
  public static Loader loader(String exportDir) {
    return new Loader(exportDir);
  }

  /**
   * Returns the serialized <a
   * href="https://www.tensorflow.org/code/tensorflow/core/protobuf/meta_graph.proto">MetaGraphDef
   * protocol buffer</a> associated with the saved model.
   */
  public byte[] metaGraphDef() {
    return metaGraphDef;
  }

  /** Returns the graph that describes the computation performed by the model. */
  public Graph graph() {
    return graph;
  }

  /**
   * Returns the {@link Session} with which to perform computation using the model.
   *
   * @return the initialized session
   */
  public Session session() {
    return session;
  }

  /**
   * Releases resources (the {@link Graph} and {@link Session}) associated with the saved model
   * bundle.
   */
  @Override
  public void close() {
    session.close();
    graph.close();
  }

  private final Graph graph;
  private final Session session;
  private final byte[] metaGraphDef;

  private SavedModelBundle(Graph graph, Session session, byte[] metaGraphDef) {
    this.graph = graph;
    this.session = session;
    this.metaGraphDef = metaGraphDef;
  }

  /**
   * Create a SavedModelBundle object from a handle to the C TF_Graph object and to the C TF_Session
   * object, plus the serialized MetaGraphDef.
   *
   * <p>Invoked from the native load method. Takes ownership of the handles.
   */
  private static SavedModelBundle fromHandle(
      long graphHandle, long sessionHandle, byte[] metaGraphDef) {
    Graph graph = new Graph(graphHandle);
    Session session = new Session(graph, sessionHandle);
    return new SavedModelBundle(graph, session, metaGraphDef);
  }

  private static native SavedModelBundle load(
      String exportDir, String[] tags, byte[] config, byte[] runOptions);

  static {
    TensorFlow.init();
  }
}
