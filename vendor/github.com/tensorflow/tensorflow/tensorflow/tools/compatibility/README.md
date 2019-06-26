# TensorFlow Python API Upgrade Utility

This tool allows you to upgrade your existing TensorFlow Python scripts.
Specifically: \
`tf_upgrade_v2.py`: upgrades code from TensorFlow 1.12 to TensorFlow 2.0 preview. \
`tf_upgrade.py`: upgrades code to TensorFlow 1.0 from TensorFlow 0.11.

## Running the script from pip package

First, install TensorFlow pip package*. See
https://www.tensorflow.org/install/pip.

Upgrade script can be run on a single Python file:

```
tf_upgrade_v2 --infile foo.py --outfile foo-upgraded.py
```

It will print a list of errors it finds that it can't fix. You can also run
it on a directory tree:

```
# upgrade the .py files and copy all the other files to the outtree
tf_upgrade_v2 --intree coolcode --outtree coolcode-upgraded

# just upgrade the .py files
tf_upgrade_v2 --intree coolcode --outtree coolcode-upgraded --copyotherfiles False
```

*Note: `tf_upgrade_v2` is installed automatically as a script by the pip install 
after TensorFlow 1.12.

## Report

The script will also dump out a report e.g. which will detail changes
e.g.:

```
'tensorflow/tools/compatibility/testdata/test_file_v1_12.py' Line 65
--------------------------------------------------------------------------------

Added keyword 'input' to reordered function 'tf.argmax'
Renamed keyword argument from 'dimension' to 'axis'

    Old:         tf.argmax([[1, 3, 2]], dimension=0))
                                        ~~~~~~~~~~
    New:         tf.argmax(input=[[1, 3, 2]], axis=0))

```

## Caveats

- Don't update parts of your code manually before running this script. In
particular, functions that have had reordered arguments like `tf.argmax`
or `tf.batch_to_space` will cause the script to incorrectly add keyword
arguments that mismap arguments.

- This script wouldn't actually reorder arguments. Instead, the script will add
keyword arguments to functions that had their arguments reordered.

- This script is not able to upgrade all functions. One notable example is
`tf.nn.conv2d` that no longer takes `use_cudnn_on_gpu` argument.
If the script detects this, it will report this to stdout
(and in the report), and you can fix it manually. For example if you have
`tf.nn.conv2d(inputs, filters, strides, padding, use_cudnn_on_gpu=True)`
you will need to manually change it to
`tf.nn.conv2d(input, filters, strides, padding)`.

- There are some syntaxes that are not handleable with this script as this
script was designed to use only standard python packages.
There is an alternative available for TensorFlow 0.* to 1.0 upgrade script.
If the script fails with "A necessary keyword argument failed to be inserted." or
"Failed to find keyword lexicographically. Fix manually.", you can try
[@machrisaa's fork of this script](https://github.com/machrisaa/tf0to1).
[@machrisaa](https://github.com/machrisaa) has used the
[RedBaron Python refactoring engine](https://redbaron.readthedocs.io/en/latest/)
which is able to localize syntactic elements more reliably than the built-in
`ast` module this script is based upon. Note that the alternative script is not
available for TensorFlow 2.0 upgrade.
