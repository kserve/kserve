import { async, ComponentFixture, TestBed } from '@angular/core/testing';

import { LogsViewerComponent } from './logs-viewer.component';

describe('LogsViewerComponent', () => {
  let component: LogsViewerComponent;
  let fixture: ComponentFixture<LogsViewerComponent>;

  beforeEach(async(() => {
    TestBed.configureTestingModule({
      declarations: [ LogsViewerComponent ]
    })
    .compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(LogsViewerComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
