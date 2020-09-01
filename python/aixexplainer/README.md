# AIX Model Explainer

## Build a Development AIX Model Explainer Docker Image

First build your docker image by changing directory to kfserving/python and replacing `dockeruser` with your docker username in the snippet below (running this will take some time).

`docker build -t dockeruser/aixserver:latest -f aixexplainer.Dockerfile .`

Then push your docker image to your dockerhub repo (this will take some time)

`docker push dockeruser/aixserver:latest`

Once your docker image is pushed you can pull the image from `dockeruser/aixserver:latest` when deploying an inferenceservice by specifying the image in the yaml file.

## Example 

Try deploying [LIME with MNIST](../../docs/samples/explanation/aix/mnist)
