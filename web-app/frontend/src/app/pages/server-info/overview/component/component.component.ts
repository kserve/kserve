import { Component, Input, OnInit } from '@angular/core';
import { getK8sObjectStatus } from 'src/app/shared/utils';
import { ComponentStatusSpec } from 'src/app/types/kfserving/v1beta1';
import { ComponentOwnedObjects } from 'src/app/types/backend';

@Component({
  selector: 'app-serving-component-overview',
  templateUrl: './component.component.html',
  styleUrls: ['./component.component.scss'],
})
export class ComponentOverviewComponent implements OnInit {
  @Input() componentName: string;
  @Input() ownedObjs: ComponentOwnedObjects;
  @Input() status: ComponentStatusSpec;

  public getStatus = getK8sObjectStatus;

  ngOnInit() {}
}
