import { Injectable } from '@angular/core';
import { environment } from 'src/environments/environment';
import { HttpClient } from '@angular/common/http';
import { catchError, map, tap, switchAll, concatAll } from 'rxjs/operators';
import { BackendService, SnackBarService } from 'kubeflow';
import { ReplaySubject, Observable, of, throwError } from 'rxjs';
import { MappingsContext } from 'source-list-map';
import { GrafanaDashboard } from '../types/grafana';

@Injectable({
  providedIn: 'root',
})
export class GrafanaService extends BackendService {
  public orgId = 1;
  public serviceInitializedSuccessfully$ = new ReplaySubject<boolean>(1);

  private dashboardsInitialized = false;
  private dashboards = new ReplaySubject<GrafanaDashboard[]>(1);
  private dashboardUris = new ReplaySubject<{
    [uri: string]: GrafanaDashboard;
  }>(1);

  constructor(http: HttpClient, snack: SnackBarService) {
    super(http, snack);

    console.log('Fetching Grafana dashboards info');
    this.getDashboardsInfo().subscribe(
      (dashboards: GrafanaDashboard[]) => {
        console.log('Fetched dashboards');
        this.dashboards.next(dashboards);

        // create a dict with URIs as key for fast lookup
        const uris = {};
        for (const ds of dashboards) {
          uris[ds.uri] = ds;
        }
        this.dashboardUris.next(uris);

        this.dashboardsInitialized = true;
        this.serviceInitializedSuccessfully$.next(true);
      },
      error => {
        console.warn(`Couldn't fetch the list of Grafana Dashboards: ${error}`);
        this.serviceInitializedSuccessfully$.next(false);
      },
    );
  }

  public getDasbhboardUrlFromUri(uri: string): Observable<string> {
    return this.withServiceInitialized().pipe(
      map(_ => this.dashboardUris),
      concatAll(),
      map(uris => {
        if (!(uri in uris)) {
          const msg = `Grafana URI '${uri}' does not exist in list of known URIs`;
          throw msg;
        }

        return uris[uri].url;
      }),
    );
  }

  public getDashboardsInfo() {
    const url = environment.grafanaPrefix + '/api/search';

    return this.http.get<GrafanaDashboard[]>(url).pipe(
      catchError(error => this.handleError(error, false)),
      map((resp: GrafanaDashboard[]) => {
        return resp;
      }),
    );
  }

  private withServiceInitialized(): Observable<boolean> {
    return this.serviceInitializedSuccessfully$.pipe(
      map(init => {
        if (!init) {
          const msg = 'Initialization process was not completed successfully.';
          throw msg;
        }

        return true;
      }),
    );
  }
}
