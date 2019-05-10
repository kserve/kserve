from setuptools import setup, find_packages

tests_require = [
        'pytest',
        'pytest-tornasync',
        'mypy'
    ]


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
        "kfserver==0.1.0",
        "xgboost == 0.82",
        "scikit-learn == 0.20.3",
        "argparse >= 1.4.0"
    ],
    tests_require=tests_require,
    extras_require={'test': tests_require}
)
