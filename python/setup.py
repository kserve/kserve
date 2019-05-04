from setuptools import setup, find_packages
import os

setup(
    name='kfserver',
    version='0.1.0',
    author_email='ellisbigelow@google.com',
    license='../../LICENSE.txt',
    url='https://github.com/kubeflow/kfserving/python/kfserving/kfserving',
    description='Model Server for arbitrary python ML frameworks.',
    long_description=open('README.md').read(),
    python_requires='>3.4',
    packages=find_packages("kfserving"),
    install_requires=[
        "tornado >= 1.4.1",
        "argparse >= 1.4.0"
    ],
)

setup(
    name='xgbserver',
    version='0.1.0',
    author_email='ellisbigelow@google.com',
    license='../../LICENSE.txt',
    url='https://github.com/kubeflow/kfserving/python/kfserving/xgbserver',
    description='Model Server implementation for XGBoost. Not intended for use outside KFServing Frameworks Images',
    long_description=open('README.md').read(),
    python_requires='>3.4',
    packages=find_packages("xgbserver"),
    install_requires=[
        "tornado >= 1.4.1",
        "xgboost == 0.82",
        "scikit-learn == 0.20.3",
        "argparse >= 1.4.0"
    ],
)
