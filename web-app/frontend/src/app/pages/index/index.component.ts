import { Component, OnInit, OnDestroy } from '@angular/core';
import { MWABackendService } from 'src/app/services/backend.service';
import { Clipboard } from '@angular/cdk-experimental/clipboard';
import {
  PredictorSpec,
  InferenceServiceK8s,
  InferenceServiceIR,
} from 'src/app/types/kfserving/v1beta1';
import { environment } from 'src/environments/environment';
import {
  NamespaceService,
  ExponentialBackoff,
  STATUS_TYPE,
  ActionEvent,
  ConfirmDialogService,
  DIALOG_RESP,
  SnackBarService,
  SnackType,
  DashboardState,
} from 'kubeflow';
import { Subscription } from 'rxjs';
import { isEqual } from 'lodash';
import { defaultConfig, generateDeleteConfig } from './config';
import { Router } from '@angular/router';
import {
  getPredictorType,
  getK8sObjectUiStatus,
  getPredictorExtensionSpec,
} from 'src/app/shared/utils';

@Component({
  selector: 'app-index',
  templateUrl: './index.component.html',
})
export class IndexComponent implements OnInit, OnDestroy {
  public env = environment;
  public currNamespace = '';
  public inferenceServices: InferenceServiceIR[] = [];
  public poller: ExponentialBackoff;
  public subs = new Subscription();
  public config = defaultConfig;
  public dashboardDisconnectedState = DashboardState.Disconnected;

  private rawData: InferenceServiceK8s[] = [];

  constructor(
    private backend: MWABackendService,
    private confirmDialog: ConfirmDialogService,
    private snack: SnackBarService,
    private router: Router,
    private clipboard: Clipboard,
    public ns: NamespaceService,
  ) {}

  ngOnInit() {
    this.poller = new ExponentialBackoff({
      interval: 1000,
      retries: 3,
      maxInterval: 4000,
    });

    this.subs.add(
      this.poller.start().subscribe(() => {
        if (!this.currNamespace) {
          return;
        }

        this.backend
          .getInferenceServices(this.currNamespace)
          .subscribe((svcs: InferenceServiceK8s[]) => {
            if (isEqual(this.rawData, svcs)) {
              return;
            }

            this.inferenceServices = this.processIncomingData(svcs);
            this.rawData = svcs;
            this.poller.reset();
          });
      }),
    );

    // Reset the poller whenever the selected namespace changes
    this.subs.add(
      this.ns.getSelectedNamespace().subscribe(ns => {
        this.currNamespace = ns;
        this.poller.reset();
      }),
    );
  }

  ngOnDestroy() {
    this.subs.unsubscribe();
    this.poller.stop();
  }

  // action handling functions
  public reactToAction(a: ActionEvent) {
    const svc = a.data as InferenceServiceIR;

    switch (a.action) {
      case 'newResourceButton':
        this.router.navigate(['/new']);
        break;
      case 'delete':
        this.deleteClicked(svc);
        break;
      case 'copy-link':
        console.log(`Copied to clipboard: ${svc.status.url}`);
        this.clipboard.copy(svc.status.url);
        this.snack.open(`Copied: ${svc.status.url}`, SnackType.Info, 4000);
        break;
      case 'name:link':
        /*
         * don't allow the user to navigate to the details page of a server
         * that is being deleted
         */
        if (svc.metadata.deletionTimestamp) {
          this.snack.open(
            'Model server is being deleted, cannot show details.',
            SnackType.Info,
            4000,
          );

          return;
        }

        this.router.navigate([
          '/details/',
          this.currNamespace,
          a.data.metadata.name,
        ]);
        break;
    }
  }

  private deleteClicked(svc: InferenceServiceIR) {
    const config = generateDeleteConfig(svc);

    const dialogRef = this.confirmDialog.open('Model server', config);
    const applyingSub = dialogRef.componentInstance.applying$.subscribe(
      applying => {
        if (!applying) {
          return;
        }

        this.backend.deleteInferenceService(svc).subscribe(
          res => {
            this.poller.reset();
            dialogRef.close(DIALOG_RESP.ACCEPT);
          },
          err => {
            config.error = err;
            dialogRef.componentInstance.applying$.next(false);
          },
        );
      },
    );

    dialogRef.afterClosed().subscribe(res => {
      applyingSub.unsubscribe();

      if (res !== DIALOG_RESP.ACCEPT) {
        return;
      }

      svc.ui.status.phase = STATUS_TYPE.TERMINATING;
      svc.ui.status.message = 'Preparing to delete Model server...';
    });
  }

  // functions for converting the response InferenceServices to the
  // Internal Representation objects
  private processIncomingData(svcs: InferenceServiceK8s[]) {
    const svcsCopy: InferenceServiceIR[] = JSON.parse(JSON.stringify(svcs));

    for (const svc of svcsCopy) {
      this.parseInferenceService(svc);
    }

    return svcsCopy;
  }

  private parseInferenceService(svc: InferenceServiceIR) {
    svc.ui = { actions: {} };
    svc.ui.status = getK8sObjectUiStatus(svc);
    svc.ui.actions.copy = this.getCopyActionStatus(svc);
    svc.ui.actions.delete = this.getDeletionActionStatus(svc);

    const predictor = getPredictorExtensionSpec(svc.spec.predictor);

    svc.ui.predictorType = getPredictorType(svc.spec.predictor);
    svc.ui.runtimeVersion = predictor.runtimeVersion;
    svc.ui.storageUri = predictor.storageUri;
    svc.ui.protocolVersion = predictor.protocolVersion;
  }

  private getCopyActionStatus(svc: InferenceServiceIR) {
    if (svc.ui.status.phase !== STATUS_TYPE.READY) {
      return STATUS_TYPE.UNAVAILABLE;
    }

    return STATUS_TYPE.READY;
  }

  private getDeletionActionStatus(svc: InferenceServiceIR) {
    if (svc.ui.status.phase !== STATUS_TYPE.TERMINATING) {
      return STATUS_TYPE.READY;
    }

    return STATUS_TYPE.TERMINATING;
  }

  // util functions
  public inferenceServiceTrackByFn(index: number, svc: InferenceServiceK8s) {
    return `${svc.metadata.name}/${svc.metadata.creationTimestamp}`;
  }
}
