import { NgModule } from '@angular/core';
import { Routes, RouterModule } from '@angular/router';
import { IndexComponent } from './pages/index/index.component';
import { ServerInfoComponent } from './pages/server-info/server-info.component';
import { SubmitFormComponent } from './pages/submit-form/submit-form.component';

const routes: Routes = [
  { path: '', component: IndexComponent },
  { path: 'details/:namespace/:name', component: ServerInfoComponent },
  { path: 'new', component: SubmitFormComponent},
];

@NgModule({
  imports: [RouterModule.forRoot(routes)],
  exports: [RouterModule],
})
export class AppRoutingModule {}
