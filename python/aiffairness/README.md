# AIF Model Fairness / Bias Detection

## Input parameters

| Name                | Description                                                                                                                                                                             |
| ------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| model_name          | (str) The name that the model is served under                                                                                                                                           |
| predictor_host      | (str) The host for the predictor.                                                                                                                                                       |
| feature_names       | (list(str)): Names describing each dataset feature.                                                                                                                                     |
| label_names         | (list(str)): Names describing each label.                                                                                                                                               |
| favorable_label     | (float): Label value which is considered favorable (i.e. "positive").                                                                                                                   |
| unfavorable_label   | (float): Label value which is considered unfavorable (i.e. "negative").                                                                                                                 |
| privileged_groups   | (list(dict)): Privileged groups in a list of `dicts` where the keys are `protected_attribute_names` and the values are values in `protected_attributes`. Each `dict` is a single group. |
| unprivileged_groups | (list(dict)): Unprivileged groups in the same format as `privileged_groups`.                                                                                                            |

## Output metrics

| Name                          | Description                                                                                                 |
| ----------------------------- | ----------------------------------------------------------------------------------------------------------- |
| base_rate                     | (float): Compute the base rate, 𝑃𝑟(𝑌=1)=𝑃/(𝑃+𝑁)                                                             |
| consistency                   | (list): Individual fairness metric from [1] that measures how similar the labels are for similar instances. |
| disparate_impact              | (float): 𝑃𝑟(𝑌=1 \| 𝐷=unprivileged)𝑃𝑟(𝑌=1 \| 𝐷=privileged)                                                   |
| num_instances                 | (float): Compute the number of instances n                                                                  |
| num_negatives                 | (float): Compute the number of negatives, 𝑁=∑𝑛𝑖=1𝟙[𝑦𝑖=0]                                                    |
| num_positives                 | (float): Compute the number of positives, 𝑃=∑𝑛𝑖=1𝟙[𝑦𝑖=1]                                                    |
| statistical_parity_difference | (float): 𝑃𝑟(𝑌=1\|𝐷=unprivileged)−𝑃𝑟(𝑌=1\|𝐷=privileged)                                                      |

[1] R. Zemel, Y. Wu, K. Swersky, T. Pitassi, and C. Dwork, “Learning Fair Representations,” International Conference on Machine Learning, 2013.

## Build a development AIF bias detector docker image

First build your docker image by changing directory to kserve/python and replacing dockeruser with your docker username in the snippet below (running this will take some time).

```
docker build -t dockeruser/aifserver:latest -f aiffairness.Dockerfile .
```

Then push your docker image to your dockerhub repo (this will take some time)

```
docker push dockeruser/aifserver:latest
```

Once your docker image is pushed you can pull the image from dockeruser/aifserver:latest when deploying an inferenceservice by specifying the image in the yaml file.
