import { async, ComponentFixture, TestBed } from '@angular/core/testing';

import { TransformerComponent } from './transformer.component';

describe('TransformerComponent', () => {
  let component: TransformerComponent;
  let fixture: ComponentFixture<TransformerComponent>;

  beforeEach(async(() => {
    TestBed.configureTestingModule({
      declarations: [ TransformerComponent ]
    })
    .compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(TransformerComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
