export interface GrafanaDashboard {
  id: number;
  uid: string;
  title: string;
  uri: string;
  url: string;
  slug: string;
  type: string;
  tags: string[];
  isStarted: boolean;
}

export interface GrafanaIframeConfig {
  panelId: number;
  width: number;
  height: number;
  dashboardUri: string;

  componentName?: string;
  refresh?: number;
  vars?: { [varName: string]: any };
}
