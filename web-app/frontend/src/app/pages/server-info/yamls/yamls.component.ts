import { Component, Input } from '@angular/core';
import { load, dump } from 'js-yaml';
import { InferenceServiceK8s } from 'src/app/types/kfserving/v1beta1';

@Component({
  selector: 'app-yamls',
  templateUrl: './yamls.component.html',
  styleUrls: ['./yamls.component.scss'],
})
export class YamlsComponent {
  @Input() svc: InferenceServiceK8s;

  get data() {
    if (!this.svc) {
      return 'No data has been found...';
    }

    return dump(this.svc);
  }
}
