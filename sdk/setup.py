import setuptools

REQUIRES = [
    'certifi >= 14.05.14',
    'six >= 1.10',
    'python_dateutil >= 2.5.3',
    'setuptools >= 21.0.0',
    'urllib3 >= 1.15.1',
    'kubernetes >= 9.0.0',
]

setuptools.setup(
    name='kfserving',
    version='0.0.1',
    author="Kubeflow Authors",
    description="Python SDK for KFServing",
    url="https://github.com/kubeflow/kfserving",
    packages=[
        'kfserving.api',
        'kfserving.constants',
        'kfserving.models',
        'kfserving.utils'
    ],
    package_data={},
    include_package_data=False,
    zip_safe=False,
    classifiers=[
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
    ],
    install_requires=REQUIRES,
)
