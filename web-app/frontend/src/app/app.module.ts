import { BrowserModule } from '@angular/platform-browser';
import { NgModule } from '@angular/core';

import { AppRoutingModule } from './app-routing.module';
import { AppComponent } from './app.component';
import { IndexModule } from './pages/index/index.module';
import { KubeflowModule } from 'kubeflow';
import { ServerInfoModule } from './pages/server-info/server-info.module';
import { SubmitFormModule } from './pages/submit-form/submit-form.module';

@NgModule({
  declarations: [AppComponent],
  imports: [
    BrowserModule,
    AppRoutingModule,
    IndexModule,
    KubeflowModule,
    ServerInfoModule,
    SubmitFormModule,
  ],
  providers: [],
  bootstrap: [AppComponent],
})
export class AppModule {}
