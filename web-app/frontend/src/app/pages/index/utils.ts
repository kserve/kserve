import { InferenceServiceK8s } from 'src/app/types/kfserving/v1beta1';
import { getPredictorExtensionSpec } from 'src/app/shared/utils';

export function parseRuntime(svc: InferenceServiceK8s): string {
  const pred = getPredictorExtensionSpec(svc.spec.predictor);

  if (pred === null) {
    return '';
  }

  return pred.runtimeVersion;
}
