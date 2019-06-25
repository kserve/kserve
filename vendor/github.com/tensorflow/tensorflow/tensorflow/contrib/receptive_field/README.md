# Receptive field computation for convnets

This library enables you to easily compute the receptive field parameters of
your favorite convnet. You can use it to understand how big of an input image
region your output features depend on. Better yet, using the parameters computed
by the library, you can easily find the exact image region which is used to
compute each convnet feature.

This library can be used to compute receptive field parameters of popular
convnets:

<center>

convnet model       | receptive field | effective stride | effective padding
:-----------------: | :-------------: | :--------------: | :---------------:
alexnet_v2          | 195             | 32               | 64
vgg_16              | 212             | 32               | 90
inception_v2        | 699             | 32               | 318
inception_v3        | 1311            | 32               | 618
inception_v4        | 2071            | 32               | 998
inception_resnet_v2 | 3039            | 32               | 1482
mobilenet_v1        | 315             | 32               | 126
mobilenet_v1_075    | 315             | 32               | 126
resnet_v1_50        | 483             | 32               | 241
resnet_v1_101       | 1027            | 32               | 513
resnet_v1_152       | 1507            | 32               | 753
resnet_v1_200       | 1763            | 32               | 881

</center>

A comprehensive table with pre-computed receptive field parameters for different
end-points, input resolutions, and other variants of these networks can be found
[here](https://github.com/tensorflow/tensorflow/blob/master/tensorflow/contrib/receptive_field/RECEPTIVE_FIELD_TABLE.md).

## Basic usage

The main function to be called is `compute_receptive_field_from_graph_def`,
which will return the receptive field, effective stride and effective padding
for both horizontal and vertical directions.

For example, if your model is constructed using the function
`my_model_construction()`, you can use the library as follows:

```python
import tensorflow as tf

# Construct graph.
g = tf.Graph()
with g.as_default():
  images = tf.placeholder(tf.float32, shape=(1, None, None, 3), name='input_image')
  my_model_construction(images)

# Compute receptive field parameters.
rf_x, rf_y, eff_stride_x, eff_stride_y, eff_pad_x, eff_pad_y = \
  tf.contrib.receptive_field.compute_receptive_field_from_graph_def( \
    g.as_graph_def(), 'input_image', 'my_output_endpoint')
```

Here's a simple example of computing the receptive field parameters for
Inception-Resnet-v2. To get this to work, be sure to checkout
[tensorflow/models](https://github.com/tensorflow/models), so that the Inception
models are available to you. This can be done in three simple commands:

```sh
git clone https://github.com/tensorflow/models
cd models/research/slim
sudo python setup.py install_lib
```

You can then compute the receptive field parameters for Inception-Resnet-v2 as:

```python
from nets import inception
import tensorflow as tf

# Construct graph.
g = tf.Graph()
with g.as_default():
  images = tf.placeholder(tf.float32, shape=(1, None, None, 3), name='input_image')
  inception.inception_resnet_v2_base(images)

# Compute receptive field parameters.
rf_x, rf_y, eff_stride_x, eff_stride_y, eff_pad_x, eff_pad_y = \
  tf.contrib.receptive_field.compute_receptive_field_from_graph_def( \
    g.as_graph_def(), 'input_image', 'InceptionResnetV2/Conv2d_7b_1x1/Relu')
```

This will give you `rf_x = rf_y = 3039`, `eff_stride_x = eff_stride_y = 32`, and
`eff_pad_x = eff_pad_y = 1482`. This means that each feature that is output at
the node `'InceptionResnetV2/Conv2d_7b_1x1/Relu'` is computed from a region
which is of size `3039x3039`. Further, by using the expressions

```python
center_x = -eff_pad_x + feature_x*eff_stride_x + (rf_x - 1)/2
center_y = -eff_pad_y + feature_y*eff_stride_y + (rf_y - 1)/2
```

one can compute the center of the region in the input image that is used to
compute the output feature at position `[feature_x, feature_y]`. For example,
the feature at position `[0, 2]` at the output of the layer
`'InceptionResnetV2/Conv2d_7b_1x1/Relu'` is centered in the original image in
the position `[37, 101]`.

TODO: include link to derivations and definitions of different parameters.

## Receptive field benchmark

As you might expect, it is straightforward to run this library on the popular
convnets, and gather their receptive fields. We provide a python script which
does exactly that, available under `python/util/examples/rf_benchmark.py`.

To get this to work, be sure to checkout
[tensorflow/models](https://github.com/tensorflow/models) (see the 3-command
instructions for this above). Then, simply:

```sh
cd python/util/examples
python rf_benchmark.py --csv_path /tmp/rf_benchmark_results.csv
```

The script will write to stdout the receptive field parameters for many variants
of several popular convnets: AlexNet, VGG, ResNet, Inception, Mobilenet. They
are also written to the file `/tmp/rf_benchmark_results.csv`.

A comprehensive table with pre-computed receptive field parameters for different
networks can be found
[here](https://github.com/tensorflow/tensorflow/blob/master/tensorflow/contrib/receptive_field/RECEPTIVE_FIELD_TABLE.md).

## Compute RF parameters from a graph pbtxt

We also provide a utility to compute the receptive field parameters directly
from a graph protobuf file.

Have a `graph.pbtxt` file and want to compute its receptive field parameters? We
got you covered. The only prerequisite is to install
[google/protobuf](https://github.com/google/protobuf), which you probably
already have if you're using tensorflow (otherwise, follow installation
instructions [here](https://github.com/google/protobuf/tree/master/python)).

This should work:

```sh
cd python/util/examples
python compute_rf.py \
  --graph_path /path/to/graph.pbtxt \
  --output_path /path/to/output/rf_info.txt \
  --input_node my_input_node \
  --output_node my_output_node
```

Don't know how to generate a graph protobuf file? Take a look at the
`write_inception_resnet_v2_graph.py` script, which shows how to save it for the
Inception-Resnet-v2 model:

```sh
cd python/util/examples
python write_inception_resnet_v2_graph.py --graph_dir /tmp --graph_filename graph.pbtxt
```

This will write the Inception-Resnet-v2 graph protobuf to `/tmp/graph.pbtxt`.

For completeness, here's how you would use this file to get the receptive field
parameters of the Inception-Resnet-v2 model:

```sh
cd python/util/examples
python compute_rf.py \
  --graph_path /tmp/graph.pbtxt \
  --output_path /tmp/rf_info.txt \
  --input_node input_image \
  --output_node InceptionResnetV2/Conv2d_7b_1x1/Relu
```

This will write the receptive field parameters of the model to
`/tmp/rf_info.txt`, which will look like:

```sh
Receptive field size (horizontal) = 3039
Receptive field size (vertical) = 3039
Effective stride (horizontal) = 32
Effective stride (vertical) = 32
Effective padding (horizontal) = 1482
Effective padding (vertical) = 1482
```

## Authors

Andr&eacute; Araujo (@andrefaraujo) and Mark Sandler (@marksandler)
