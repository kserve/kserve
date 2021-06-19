import { Component, Input } from '@angular/core';
import { V1PodSpec } from '@kubernetes/client-node';

@Component({
  selector: 'app-pod-details',
  templateUrl: './pod.component.html',
  styleUrls: ['./pod.component.scss'],
})
export class PodComponent {
  @Input() pod: V1PodSpec;
}
