# Generate bytes input for KServe v2 protocol

The python script converts input image into bytes

**Steps:**

 1. Check python packages are installed
 2. Run below command
 3. This will write a json file with the name of the image.

```bash
python tobytes.py 0.png
```

`0.json` file be created 

In case of bytes input, the [custom handler](https://github.com/pytorch/serve/blob/master/examples/image_classifier/mnist/mnist_handler.py) can be used without any change - 