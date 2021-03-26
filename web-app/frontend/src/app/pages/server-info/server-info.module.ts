import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ServerInfoComponent } from './server-info.component';
import { KubeflowModule } from 'kubeflow';
import { MatIconModule } from '@angular/material/icon';
import { MatDividerModule } from '@angular/material/divider';
import { MatTabsModule } from '@angular/material/tabs';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { OverviewModule } from './overview/overview.module';
import { DetailsModule } from './details/details.module';
import { MetricsModule } from './metrics/metrics.module';
import { LogsModule } from './logs/logs.module';
import { YamlsModule } from './yamls/yamls.module';

@NgModule({
  declarations: [ServerInfoComponent],
  imports: [
    CommonModule,
    KubeflowModule,
    MatIconModule,
    MatDividerModule,
    MatTabsModule,
    MatProgressSpinnerModule,
    OverviewModule,
    DetailsModule,
    MetricsModule,
    LogsModule,
    YamlsModule,
  ],
})
export class ServerInfoModule {}
