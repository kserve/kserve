import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { SubmitFormComponent } from './submit-form.component';
import { FormModule, KubeflowModule } from 'kubeflow';
import { AceEditorModule } from 'ng2-ace-editor';

@NgModule({
  declarations: [SubmitFormComponent],
  imports: [CommonModule, KubeflowModule, AceEditorModule],
  exports: [SubmitFormComponent],
})
export class SubmitFormModule {}
