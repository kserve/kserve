import { async, ComponentFixture, TestBed } from '@angular/core/testing';

import { ComponentExtensionComponent } from './component-extension.component';

describe('ComponentExtensionComponent', () => {
  let component: ComponentExtensionComponent;
  let fixture: ComponentFixture<ComponentExtensionComponent>;

  beforeEach(async(() => {
    TestBed.configureTestingModule({
      declarations: [ ComponentExtensionComponent ]
    })
    .compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(ComponentExtensionComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
