import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { MetricsComponent } from './metrics.component';
import { GrafanaGraphComponent } from './grafana-graph/grafana-graph.component';
import { KubeflowModule, HeadingSubheadingRowModule } from 'kubeflow';

@NgModule({
  declarations: [MetricsComponent, GrafanaGraphComponent],
  imports: [CommonModule, KubeflowModule, HeadingSubheadingRowModule],
  exports: [MetricsComponent],
})
export class MetricsModule {}
