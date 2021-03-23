from tensorflow.keras.applications.inception_v3 import InceptionV3
from alibi.explainers import AnchorImage
import dill

model = InceptionV3(weights='imagenet')

segmentation_fn = 'slic'
kwargs = {'n_segments': 15, 'compactness': 20, 'sigma': .5}
image_shape = (299, 299, 3)
explainer = AnchorImage(lambda x: model.predict(x), image_shape, segmentation_fn=segmentation_fn,
                        segmentation_kwargs=kwargs,
                        images_background=None)


explainer.predict_fn = None  # Clear explainer predict_fn as its a lambda and will be reset when loaded
with open("explainer.dill", 'wb') as f:
    dill.dump(explainer, f)
