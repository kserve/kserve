import matplotlib
import matplotlib.pyplot as plt
from tensorflow.keras.applications.inception_v3 import InceptionV3, preprocess_input, decode_predictions
from alibi.datasets import fetch_imagenet
import numpy as np
import requests
import json

category = 'Persian cat'
image_shape = (299, 299, 3)
data, labels = fetch_imagenet(category, nb_images=10, target_size=image_shape[:2], seed=2, return_X_y=True)
print('Images shape: {}'.format(data.shape))
images = preprocess_input(data)

payload = {
    "instances": [images[0].tolist()]
}

# sending post request to TensorFlow Serving server
r = requests.post('http://localhost:8081/models/imagenet:explain', json=payload)
explanation = json.loads(r.content.decode('utf-8'))

exp_arr = np.array(explanation['anchor'])

f, axarr = plt.subplots(1,2)
axarr[0].imshow(data[0])
axarr[1].imshow(explanation['anchor'])
plt.show()

