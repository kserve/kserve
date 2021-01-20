import { Component, Input } from '@angular/core';
import {
  PredictorSpec,
  PredictorExtensionSpec,
} from 'src/app/types/kfserving/v1beta1';
import {
  getPredictorExtensionSpec,
  getPredictorType,
} from 'src/app/shared/utils';

@Component({
  selector: 'app-predictor-details',
  templateUrl: './predictor.component.html',
})
export class PredictorDetailsComponent {
  @Input() predictorSpec: PredictorSpec;

  get basePredictor(): PredictorExtensionSpec {
    return getPredictorExtensionSpec(this.predictorSpec);
  }

  get predictorType(): string {
    return getPredictorType(this.predictorSpec);
  }
}
