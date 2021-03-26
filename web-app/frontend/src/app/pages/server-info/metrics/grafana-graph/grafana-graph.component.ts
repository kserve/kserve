import { Component, Input, HostBinding } from '@angular/core';
import { GrafanaService } from 'src/app/services/grafana.service';
import { Observable, of, throwError } from 'rxjs';
import { map, catchError } from 'rxjs/operators';
import { DomSanitizer, SafeUrl } from '@angular/platform-browser';
import { GrafanaIframeConfig } from 'src/app/types/grafana';

const defaultIframeConfig: GrafanaIframeConfig = {
  panelId: -1,
  width: 450,
  height: 200,
  dashboardUri: 'db/path',
  refresh: 5,
};

@Component({
  selector: 'app-grafana-graph',
  templateUrl: './grafana-graph.component.html',
  styleUrls: ['./grafana-graph.component.scss'],
})
export class GrafanaGraphComponent {
  public graphLoadError: string;
  public iframeUrl: SafeUrl;

  @Input()
  set config(c: GrafanaIframeConfig) {
    this.configPrv = { ...defaultIframeConfig, ...c };

    this.generateIframeUrl(this.config).subscribe(url => {
      this.iframeUrl = url;
    });
  }

  get config() {
    return this.configPrv;
  }

  private configPrv: GrafanaIframeConfig = defaultIframeConfig;

  @HostBinding('style.width.px') width = this.config.width;
  @HostBinding('style.height.px') height = this.config.height;

  constructor(
    public grafana: GrafanaService,
    private sanitizer: DomSanitizer,
  ) {}

  private generateIframeUrl(config: GrafanaIframeConfig): Observable<SafeUrl> {
    return this.grafana.getDasbhboardUrlFromUri(config.dashboardUri).pipe(
      catchError(error => {
        this.graphLoadError = error;
        return throwError(error);
      }),
      map((url: string) => {
        // replace /d/ with /d-solo/
        let iframeUrl = url.replace('/d/', '/d-solo/');

        // add the query params
        iframeUrl = `${iframeUrl}?orgId=${this.grafana.orgId}`;
        iframeUrl = `${iframeUrl}&panelId=${config.panelId}`;
        iframeUrl = `${iframeUrl}&refresh=${config.refresh}s`;
        iframeUrl = `${iframeUrl}&theme=light`;

        for (const key in config.vars) {
          if (config.vars.hasOwnProperty(key)) {
            iframeUrl = `${iframeUrl}&${key}=${config.vars[key]}`;
          }
        }

        return this.sanitizer.bypassSecurityTrustResourceUrl(iframeUrl);
      }),
    );
  }
}
