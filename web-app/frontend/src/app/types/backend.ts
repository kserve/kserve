import { BackendResponse, Status, STATUS_TYPE, K8sObject } from 'kubeflow';
import { InferenceServiceK8s } from './kfserving/v1beta1';

export interface MWABackendResponse extends BackendResponse {
  inferenceServices?: InferenceServiceK8s[];
  inferenceService?: InferenceServiceK8s;
  knativeService?: K8sObject;
  knativeConfiguration?: K8sObject;
  knativeRevision?: K8sObject;
  knativeRoute?: K8sObject;
  serviceLogs?: InferenceServiceLogs;
}

export interface InferenceServiceLogs {
  predictor?: { podName: string; logs: string[] }[];
  transformer?: { podName: string; logs: string[] }[];
  explainer?: { podName: string; logs: string[] }[];
}

// types presenting the InferenceService dependent k8s objects
export interface InferenceServiceOwnedObjects {
  predictor?: ComponentOwnedObjects;
  transformer?: ComponentOwnedObjects;
  explainer?: ComponentOwnedObjects;
}

export interface ComponentOwnedObjects {
  revision: K8sObject;
  configuration: K8sObject;
  knativeService: K8sObject;
  route: K8sObject;
}
