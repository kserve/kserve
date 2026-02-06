"""Constants for helm-converter

Centralized location for hardcoded values to improve maintainability.
"""

# Main component names that are always enabled (no conditional wrapping needed)
MAIN_COMPONENTS = ['kserve', 'llmisvc', 'localmodel', 'localmodelnode']

# Kubernetes manifest paths
# Path to pod template spec in Deployment/DaemonSet/Job
POD_TEMPLATE_SPEC_PATH = ['spec', 'template', 'spec']
# Path to containers array in pod spec (for Deployment/DaemonSet/Job)
CONTAINERS_PATH = POD_TEMPLATE_SPEC_PATH + ['containers']
# Path to containers array in ClusterServingRuntime/ServingRuntime
RUNTIME_CONTAINERS_PATH = ['spec', 'containers']
# Index of first container (default container to process)
FIRST_CONTAINER_INDEX = 0

# KServe core CRDs managed by kserve-crd chart
# These CRDs should be skipped when generating component templates
KSERVE_CORE_CRDS = {
    'clusterservingruntimes.serving.kserve.io',
    'inferencegraphs.serving.kserve.io',
    'inferenceservices.serving.kserve.io',
    'servingruntimes.serving.kserve.io',
    'trainedmodels.serving.kserve.io',
    'inferencepools.serving.kserve.io',
    'clusterstoragecontainers.serving.kserve.io',
    'localmodelnodes.serving.kserve.io',
    'localmodelnodegroups.serving.kserve.io',
    'clustertrainedmodels.serving.kserve.io'
}

# Component-specific CRDs that should go to crds/ directory
# NOTE: localmodel CRDs are managed separately by user (kserve-localmodel-crd chart)
COMPONENT_SPECIFIC_CRDS = {
    'llmisvc': {
        'inferenceobjectives.inference.networking.x-k8s.io',
        'inferencepoolimports.inference.networking.x-k8s.io',
        'inferencepools.inference.networking.k8s.io',
        'inferencepools.inference.networking.x-k8s.io'
    }
}
