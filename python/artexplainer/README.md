# ART Model Explainer

## Build your own ART Model Explainer Docker Image


First build your docker image by changing directory to kserve/python and replacing `dockeruser` with your docker username in the snippet below.


`docker build -t dockeruser/artserver:latest -f artexplainer.Dockerfile .`

Then push your docker image to your dockerhub repo


`docker push dockeruser/artserver:latest`

Once your docker image is pushed you can pull the image from `dockeruser/artserver:latest` when deploying an inferenceservice by specifying the image in the yaml file.
