# Multi-layer perceptron classifier Scikit-Learn server

This is a  scikit-learn server which uses a sklearn.MLPClassifier to make classifications on the MNIST dataset. The model was trained using an adapted version of scikit-learn's [Visualization of MLP weights on MNIST](https://scikit-learn.org/stable/auto_examples/neural_networks/plot_mnist_filters.html#sphx-glr-auto-examples-neural-networks-plot-mnist-filters-py).

## Train a new model

Move to the `kfserving/docs/samples/explanation/art/mnist` directory

`python train_model.py`

This will train a new model and put the new model in `sklearnserver/sklearnserver/example_model/model.pkl`.

To change the model adapt this line.

```
mlp = MLPClassifier(hidden_layer_sizes=(500,500,500), max_iter=10, alpha=1e-4,
                    solver='sgd', verbose=10, random_state=1,
                    learning_rate_init=.1)
```

## Build a Development MLPClassifier Scikit-Learn server Docker Image

Replace `dockeruser` with your docker username in the snippet below.

`docker build -t dockeruser/mlp-server:latest -f sklearn.Dockerfile .`

Then push your docker image to your dockerhub repo

`docker push dockeruser/mlp-server:latest`

Once your docker image is pushed you can pull the image from `dockeruser/mlp-server:latest` when deploying an inferenceservice by specifying the image in the yaml file.
