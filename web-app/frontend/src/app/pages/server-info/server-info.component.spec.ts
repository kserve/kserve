import { async, ComponentFixture, TestBed } from '@angular/core/testing';

import { ServerInfoComponent } from './server-info.component';

describe('ServerInfoComponent', () => {
  let component: ServerInfoComponent;
  let fixture: ComponentFixture<ServerInfoComponent>;

  beforeEach(async(() => {
    TestBed.configureTestingModule({
      declarations: [ ServerInfoComponent ]
    })
    .compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(ServerInfoComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
