import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { DetailsComponent } from './details.component';
import {
  DetailsListModule,
  HeadingSubheadingRowModule,
  DateTimeModule,
} from 'kubeflow';
import { PredictorDetailsComponent } from './predictor/predictor.component';
import { TransformerComponent } from './transformer/transformer.component';
import { ExplainerComponent } from './explainer/explainer.component';
import { ContainerComponent } from './shared/container/container.component';
import { ComponentExtensionComponent } from './shared/component-extension/component-extension.component';
import { PodComponent } from './shared/pod/pod.component';

@NgModule({
  declarations: [
    DetailsComponent,
    PredictorDetailsComponent,
    TransformerComponent,
    ExplainerComponent,
    ContainerComponent,
    ComponentExtensionComponent,
    PodComponent,
  ],
  imports: [
    CommonModule,
    DetailsListModule,
    HeadingSubheadingRowModule,
    DateTimeModule,
  ],
  exports: [DetailsComponent],
})
export class DetailsModule {}
