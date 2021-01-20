import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { IndexComponent } from './index.component';
import {
  NamespaceSelectModule,
  ResourceTableModule,
  ConfirmDialogModule,
} from 'kubeflow';

@NgModule({
  declarations: [IndexComponent],
  imports: [
    CommonModule,
    NamespaceSelectModule,
    ResourceTableModule,
    ConfirmDialogModule,
  ],
  exports: [],
})
export class IndexModule {}
