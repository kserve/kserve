import { Component, Input } from '@angular/core';
import { ListEntry, ChipDescriptor } from 'kubeflow';
import {
  getReadyCondition,
  getPredictorType,
  getK8sObjectStatus,
} from 'src/app/shared/utils';
import { InferenceServiceK8s } from 'src/app/types/kfserving/v1beta1';

@Component({
  selector: 'app-details',
  templateUrl: './details.component.html',
  styleUrls: ['./details.component.scss'],
})
export class DetailsComponent {
  public svcPropsList: ListEntry[] = [];
  public annotations: ChipDescriptor[] = [];
  public labels: ChipDescriptor[] = [];

  @Input() namespace: string;
  @Input()
  set svc(s: InferenceServiceK8s) {
    this.svcPrv = s;

    this.svcPropsList = this.generateSvcPropsList();
  }
  get svc(): InferenceServiceK8s {
    return this.svcPrv;
  }

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

  private svcPrv: InferenceServiceK8s;

  private generateSvcPropsList(): ListEntry[] {
    const props: ListEntry[] = [];

    this.annotations = this.generateAnnotations();
    this.labels = this.generateLabels();

    return props;
  }

  private generateAnnotations() {
    const chips = [];

    if (!this.svc.metadata.annotations) {
      return chips;
    }

    for (const a in this.svc.metadata.annotations) {
      if (!this.svc.metadata.annotations.hasOwnProperty(a)) {
        continue;
      }
      const annotationKey = a;
      const annotationVal = this.svc.metadata.annotations[a];
      if (annotationKey.includes('last-applied-configuration')) {
        continue;
      }
      const chip: ChipDescriptor = {
        value: `${annotationKey}: ${annotationVal}`,
        color: 'primary',
      };
      chips.push(chip);
    }

    return chips;
  }

  private generateLabels() {
    const chips = [];
    if (!this.svc.metadata.labels) {
      return chips;
    }

    for (const l in this.svc.metadata.labels) {
      if (!this.svc.metadata.labels.hasOwnProperty(l)) {
        continue;
      }

      const labelKey = l;
      const labelVal = this.svc.metadata.labels[l];

      const chip: ChipDescriptor = {
        value: `${labelKey}: ${labelVal}`,
        color: 'primary',
      };

      chips.push(chip);
    }

    return chips;
  }
}
