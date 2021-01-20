import { Status, STATUS_TYPE, K8sObject, Condition } from 'kubeflow';
import { V1ObjectMeta, V1Container, V1PodSpec } from '@kubernetes/client-node';

export interface InferenceServiceIR extends InferenceServiceK8s {
  // this typed is used in the frontend after parsing the backend response
  ui: {
    actions: {
      delete?: STATUS_TYPE;
      copy?: STATUS_TYPE;
    };

    status?: Status;
    runtimeVersion?: string;
    predictorType?: string;
    storageUri?: string;
    protocolVersion?: string;
  };
}

export interface InferenceServiceK8s extends K8sObject {
  metadata?: V1ObjectMeta;
  spec?: InferenceServiceSpec;
  status?: InferenceServiceStatus;
}

/**
 * Spec of InferenceService
 */
export interface InferenceServiceSpec {
  predictor: PredictorSpec;
  explainer: ExplainerSpec;
  transformer: TransformerSpec;
}

export interface PredictorSpec extends V1PodSpec, ComponentExtensionSpec {
  sklearn?: PredictorExtensionSpec;
  xgboost?: PredictorExtensionSpec;
  tensorflow?: PredictorExtensionSpec;
  pytorch?: TorchServeSpec;
  triton?: PredictorExtensionSpec;
  onnx?: PredictorExtensionSpec;
  pmml?: PredictorExtensionSpec;
  lightgbm?: PredictorExtensionSpec;
}

export interface TorchServeSpec extends PredictorExtensionSpec {
  modelClassName: string;
}

export interface PredictorExtensionSpec extends V1Container {
  storageUri?: string;
  runtimeVersion?: string;
  protocolVersion?: string;
}

export interface ComponentExtensionSpec {
  minReplicas?: number;
  maxReplicas?: number;
  containerConcurrency?: number;
  timeout?: number;
  canaryTrafficPercent?: number;
  logger?: LoggerSpec;
  batcher?: Batcher;
}

export interface LoggerSpec {
  url?: string;
  mode?: LoggerType;
}

export type LoggerType = 'all' | 'request' | 'response';

export interface Batcher {
  maxBatchSize?: number;
  maxLatency?: number;
  timeout?: number;
}
export interface ExplainerSpec extends V1PodSpec, ComponentExtensionSpec {
  alibi?: AlibiExplainerSpec;
  aix?: AIXExplainerSpec;
}

export interface AlibiExplainerSpec extends V1Container {
  type?: AlibiExplainerType;
  storageUri?: string;
  runtimeVersion?: string;
  config?: { [key: string]: string };
}

export type AlibiExplainerType =
  | 'AnchorTabular'
  | 'AnchorImages'
  | 'AnchorText'
  | 'Counterfactuals'
  | 'Contrastive';

export interface AIXExplainerSpec extends V1Container {
  type?: AIXExplainerType;
  storageUri?: string;
  runtimeVersion?: string;
  config?: { [key: string]: string };
}

export type AIXExplainerType = 'LimeImages';

export interface TransformerSpec extends V1PodSpec, ComponentExtensionSpec {}

/**
 * Status of InferenceService
 */
export interface InferenceServiceStatus extends KnativeV1Status {
  address: {
    url: string;
  };
  url: string;
  components: ComponentsStatus;
}

export interface ComponentsStatus {
  predictor: ComponentStatusSpec;
  explainer?: ComponentStatusSpec;
  transformer?: ComponentStatusSpec;
}

export interface KnativeV1Status {
  observedGeneration: number;
  conditions: Condition[];
  annotations: { [key: string]: string };
}

export interface ComponentStatusSpec {
  latestReadyRevision?: string;
  latestCreatedRevision?: string;
  previousRolledoutRevision?: string;
  latestRolledoutRevision?: string;
  traffic?: KnativeV1TrafficTarget;
  url?: string;
  address: {
    url?: string;
  };
}

export interface KnativeV1TrafficTarget {
  tag?: string;
  revisionName?: string;
  configurationName?: string;
  latestRevision?: string;
  percent?: number;
  url?: string;
}
