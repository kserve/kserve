import { Component, Input } from '@angular/core';
import { ChipDescriptor } from 'kubeflow';
import { ExplainerSpec } from 'src/app/types/kfserving/v1beta1';
import { getExplainerContainer } from 'src/app/shared/utils';

@Component({
  selector: 'app-explainer-details',
  templateUrl: './explainer.component.html',
})
export class ExplainerComponent {
  public config: ChipDescriptor[];

  @Input()
  set explainerSpec(spec: ExplainerSpec) {
    this.explainerPrv = spec;
  }
  get explainerSpec(): ExplainerSpec {
    return this.explainerPrv;
  }

  private explainerPrv: ExplainerSpec;

  private generateConfig(spec: ExplainerSpec): ChipDescriptor[] {
    const chips = [];

    for (const key in this.explainerSpec.alibi.config) {
      if (this.explainerSpec.alibi.config.hasOwnProperty(key)) {
        continue;
      }

      const val = this.explainerSpec.alibi.config[key];
      chips.push({
        value: `${key}: ${val}`,
        color: 'primary',
      });
    }

    return chips;
  }

  get container() {
    return getExplainerContainer(this.explainerSpec);
  }
}
