from setuptools import setup, find_packages

tests_require = [
    'pytest',
    'pytest-tornasync',
    'mypy'
]
setup(
    name='sklearnserver',
    version='0.1.0',
    author_email='singhan@us.ibm.com',
    license='https://github.com/kubeflow/kfserving/LICENSE',
    url='https://github.com/kubeflow/kfserving/python/sklearnserver',
    description='Model Server implementation for scikit-learn. Not intended for use outside KFServing Frameworks Images',
    long_description=open('README.md').read(),
    python_requires='>3.4',
    packages=find_packages("sklearnserver"),
    install_requires=[
        "kfserver==0.1.0",
        "scikit-learn == 0.20.3",
        "argparse >= 1.4.0",
        "numpy >= 1.8.2",
        "joblib >= 0.13.0"
    ],
    tests_require=tests_require,
    extras_require={'test': tests_require}
)
