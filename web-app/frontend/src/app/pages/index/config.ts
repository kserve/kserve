import {
  PropertyValue,
  StatusValue,
  ActionListValue,
  ActionIconValue,
  ActionButtonValue,
  TRUNCATE_TEXT_SIZE,
  MenuValue,
  DateTimeValue,
  DialogConfig,
  TableConfig,
} from 'kubeflow';
import { InferenceServiceK8s } from 'src/app/types/kfserving/v1beta1';
import { parseRuntime } from './utils';

export function generateDeleteConfig(svc: InferenceServiceK8s): DialogConfig {
  return {
    title: 'Delete Model server',
    message: `You cannot undo this action. Are you sure you want to delete this Model server?`,
    accept: 'DELETE',
    applying: 'DELETING',
    confirmColor: 'warn',
    cancel: 'cancel',
  };
}

export const defaultConfig: TableConfig = {
  title: 'Model Servers',
  newButtonText: 'NEW MODEL SERVER',
  columns: [
    {
      matHeaderCellDef: 'Status',
      matColumnDef: 'status',
      value: new StatusValue({ field: 'ui.status' }),
    },
    {
      matHeaderCellDef: 'Name',
      matColumnDef: 'name',
      value: new PropertyValue({
        field: 'metadata.name',
        truncate: TRUNCATE_TEXT_SIZE.SMALL,
        popoverField: 'metadata.name',
        isLink: true,
      }),
    },
    {
      matHeaderCellDef: 'Age',
      matColumnDef: 'age',
      value: new DateTimeValue({
        field: 'metadata.creationTimestamp',
      }),
    },
    {
      matHeaderCellDef: 'Predictor',
      matColumnDef: 'predictorType',
      value: new PropertyValue({
        field: 'ui.predictorType',
      }),
    },
    {
      matHeaderCellDef: 'Runtime',
      matColumnDef: 'runtimeVersion',
      value: new PropertyValue({
        field: 'ui.runtimeVersion',
      }),
    },
    {
      matHeaderCellDef: 'Protocol',
      matColumnDef: 'protocol',
      value: new PropertyValue({
        field: 'ui.protocolVersion',
      }),
      minWidth: '40px',
    },
    {
      matHeaderCellDef: 'Storage URI',
      matColumnDef: 'storageUri',
      value: new PropertyValue({
        field: 'ui.storageUri',
        truncate: TRUNCATE_TEXT_SIZE.MEDIUM,
        popoverField: 'ui.storageUri',
      }),
    },
    {
      matHeaderCellDef: '',
      matColumnDef: 'actions',
      value: new ActionListValue([
        new ActionIconValue({
          name: 'copy-link',
          tooltip: "Copy the server's endpoint",
          color: 'primary',
          field: 'ui.actions.copy',
          iconReady: 'material:content_copy',
        }),
        new ActionIconValue({
          name: 'delete',
          tooltip: 'Delete Server',
          color: '',
          field: 'ui.actions.delete',
          iconReady: 'material:delete',
        }),
      ]),
    },
  ],
};
