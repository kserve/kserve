
# Design Docs

 * [KFserving control plane specification](control-plane.md)
   * Defines the specification of the `kfserving` resource.
 * [KFserving data plane specification](data-plane.md)
   * Defines the data payloads to enable interoperability between `kfserving` components.

# Architecture Overview
The KFService Data Plane architecture consists of a static graph of components. The diagram may not be up to date with the current set of features available in the graph, but the overall mechanism holds:

- The User defines the components they wish to use in a KFService.
  - The only required component is a Predictor.
  - Additional components may be specified to attach addtional behavior. 
  - Each component is specified at the ModelSpec layer, allowing for canarying.
- The KFService will support one of several Data Plane interfaces.
  - For example: cloud-ai-platform/HTTP, cloud-ai-platform/GRPC, seldon/HTTP, seldon/GRPC
  - Data Plane interfaces may be specified with annotations and defaults to cloud-ai-platform/HTTP
  - Not all components will be compatible with every data plane interface.
- The Orchestrator wires up the components.
  - If the Predictor is the only component, the orchestrator will not be deployed.
  - The Orchestrator will orchestrate requests against the existing components.
- The Orchestrator will return a final response once the results have been aggregated.
  - Some components (e.g. payload logging) will execute asyncronously.

![Data Plane](./diagrams/dataplane.jpg)