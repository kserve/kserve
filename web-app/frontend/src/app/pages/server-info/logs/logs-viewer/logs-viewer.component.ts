import {
  Component,
  Input,
  ViewChild,
  NgZone,
  SimpleChanges,
  OnChanges,
  HostBinding,
  ElementRef,
  AfterViewInit,
} from '@angular/core';
import { CdkVirtualScrollViewport } from '@angular/cdk/scrolling';
import { take } from 'rxjs/operators';

@Component({
  selector: 'app-logs-viewer',
  templateUrl: './logs-viewer.component.html',
  styleUrls: ['./logs-viewer.component.scss'],
})
export class LogsViewerComponent implements AfterViewInit {
  @HostBinding('class.app-logs-viewer') selfClass = true;
  @ViewChild(CdkVirtualScrollViewport, { static: true })
  viewPort: CdkVirtualScrollViewport;

  @Input() heading = 'Logs';
  @Input() subHeading = 'tit';
  @Input() height = '400px';
  @Input()
  set logs(newLogs: string[]) {
    const currLogsLength = this.logs.length;

    this.logsPrv = newLogs;

    if (!this.logs) {
      return;
    }

    if (currLogsLength === 0) {
      setTimeout(() => {
        const scrollIndex = this.logsPrv.length - 1;
        this.viewPort.scrollToIndex(scrollIndex, 'smooth');
      }, 350);
    }
  }
  get logs(): string[] {
    return this.logsPrv;
  }

  private logsPrv: string[] = [];

  ngAfterViewInit() {
    this.viewPort.elementRef.nativeElement.style.height = this.height;
  }
}
