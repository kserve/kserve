import { Component, Input } from '@angular/core';
import {
  NamespaceService,
  ListEntry,
  ChipDescriptor,
  K8sObject,
} from 'kubeflow';
import {
  getReadyCondition,
  getPredictorType,
  getK8sObjectStatus,
  getPredictorExtensionSpec,
} from 'src/app/shared/utils';
import {
  InferenceServiceK8s,
  PredictorSpec,
  PredictorExtensionSpec,
} from 'src/app/types/kfserving/v1beta1';
import { InferenceServiceOwnedObjects } from 'src/app/types/backend';

@Component({
  selector: 'app-overview',
  templateUrl: './overview.component.html',
  styleUrls: ['./overview.component.scss'],
})
export class OverviewComponent {
  public svcPropsList: ListEntry[] = [];
  public components: ChipDescriptor[];

  @Input() namespace: string;
  @Input()
  set svc(s: InferenceServiceK8s) {
    this.svcPrv = s;

    this.components = this.generateDefaultComponents(this.svc);
  }
  get svc(): InferenceServiceK8s {
    return this.svcPrv;
  }

  @Input() ownedObjects: InferenceServiceOwnedObjects;

  private svcPrv: InferenceServiceK8s;

  get status() {
    return getK8sObjectStatus(this.svc)[0];
  }

  get statusIcon() {
    return getK8sObjectStatus(this.svc)[1];
  }

  get externalUrl() {
    if (!this.svc.status) {
      return 'InferenceService is not ready to recieve traffic yet.';
    }

    return this.svc.status.url !== undefined
      ? this.svc.status.url
      : 'InferenceService is not ready to recieve traffic yet.';
  }

  get internalUrl() {
    const msg = 'InferenceService is not ready to recieve traffic yet.';

    if (!this.svc.status || !this.svc.status.address) {
      return msg;
    }

    return this.svc.status.address.url !== undefined
      ? this.svc.status.address.url
      : msg;
  }

  get predictor(): PredictorSpec {
    return this.svc.spec.predictor;
  }

  get basePredictor(): PredictorExtensionSpec {
    return getPredictorExtensionSpec(this.svc.spec.predictor);
  }

  get predictorType(): string {
    return getPredictorType(this.svc.spec.predictor);
  }

  private generateDefaultComponents(
    svc: InferenceServiceK8s,
  ): ChipDescriptor[] {
    const chips = [];

    for (const c of ['predictor', 'transformer', 'explainer']) {
      if (c in svc.spec) {
        chips.push({
          value: c,
          color: 'primary',
        });
      }
    }

    return chips;
  }
}
