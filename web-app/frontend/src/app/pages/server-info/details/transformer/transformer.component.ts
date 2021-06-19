import { Component, Input } from '@angular/core';
import { TransformerSpec } from 'src/app/types/kfserving/v1beta1';

@Component({
  selector: 'app-transformer-details',
  templateUrl: './transformer.component.html',
})
export class TransformerComponent {
  @Input() transformerSpec: TransformerSpec;
}
