import { async, ComponentFixture, TestBed } from '@angular/core/testing';

import { ComponentComponent } from './component.component';

describe('ComponentComponent', () => {
  let component: ComponentComponent;
  let fixture: ComponentFixture<ComponentComponent>;

  beforeEach(async(() => {
    TestBed.configureTestingModule({
      declarations: [ ComponentComponent ]
    })
    .compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(ComponentComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
