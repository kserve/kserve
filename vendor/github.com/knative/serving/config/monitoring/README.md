# Monitoring Deployment

This folder contains deployment files for monitoring and logging components.

## Tracing

Deployment files are available for a range of distributed tracing solutions. However, only one solution can be deployed at a time. Refer to the following links to find out more information on capabilities and benefits of each solution.

* [Zipkin](https://zipkin.io/)
* [Jaeger](https://www.jaegertracing.io/)

## Notes for Contributors

`kubectl -R -f` installs the files within a folder in alphabetical order. In
order to install the files with correct ordering within a folder, a three digit
prefix is added.

- Files with a prefix require files with smaller prefixes to be installed before
  they are installed.
- Files with the same prefix can be installed in any order within the set
  sharing the same prefix.
- Files without any prefix can be installed in any order and they don't have any
  dependencies.
