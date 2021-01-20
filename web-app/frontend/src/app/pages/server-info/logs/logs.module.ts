import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { LogsComponent } from './logs.component';
import { KubeflowModule, PanelModule, LoadingSpinnerModule } from 'kubeflow';
import { LogsViewerModule } from './logs-viewer/logs-viewer.module';

@NgModule({
  declarations: [LogsComponent],
  imports: [
    CommonModule,
    KubeflowModule,
    LogsViewerModule,
    LoadingSpinnerModule,
    PanelModule,
  ],
  exports: [LogsComponent],
})
export class LogsModule {}
