import setuptools

with open('requirements.txt') as f:
    requirements = f.read().splitlines()

setuptools.setup(
    name='kfserving',
    version='0.1',
    author="Kubeflow Authors",
    description="Python SDK for KFServing",
    url="https://github.com/kubeflow/kfserving",
    packages=[
        'kfserving.api',
        'kfserving.constants',
        'kfserving.models',
    ],
    package_data={},
    include_package_data=False,
    zip_safe=False,
    classifiers=(
        'Intended Audience :: Developers',
        'Intended Audience :: Education',
        'Intended Audience :: Science/Research',
        'Programming Language :: Python :: 3',
        'Programming Language :: Python :: 3.5',
        'Programming Language :: Python :: 3.6',
        'Programming Language :: Python :: 3.7',
        "License :: OSI Approved :: Apache Software License",
        "Operating System :: OS Independent",
        'Topic :: Scientific/Engineering',
        'Topic :: Scientific/Engineering :: Artificial Intelligence',
        'Topic :: Software Development',
        'Topic :: Software Development :: Libraries',
        'Topic :: Software Development :: Libraries :: Python Modules',
    ),
    install_requires=requirements,
)
