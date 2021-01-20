import { async, ComponentFixture, TestBed } from '@angular/core/testing';

import { GrafanaGraphComponent } from './grafana-graph.component';

describe('GrafanaGraphComponent', () => {
  let component: GrafanaGraphComponent;
  let fixture: ComponentFixture<GrafanaGraphComponent>;

  beforeEach(async(() => {
    TestBed.configureTestingModule({
      declarations: [ GrafanaGraphComponent ]
    })
    .compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(GrafanaGraphComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
