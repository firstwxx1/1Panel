import http from '@/api';

export interface TamperSite {
    alias: string;
    domain: string;
    root: string;
    enabled: boolean;
    restore: boolean;
    excludes: string[];
    files: number;
    baselineAt: string;
    pending: number;
}

export interface TamperSettings {
    enabled: boolean;
    interval: number;
    sites: TamperSite[];
}

export interface TamperEvent {
    id: number;
    time: string;
    site: string;
    kind: 'modified' | 'added' | 'deleted';
    path: string;
    action: string;
}

const base = '/xpack/tamper';

export const tamperGetSettings = () => http.get<TamperSettings>(`${base}/settings`);
export const tamperSaveGlobal = (req: { enabled: boolean; interval: number }) => http.post(`${base}/settings`, req);
export const tamperSaveSite = (req: { alias: string; enabled: boolean; restore: boolean; excludes: string[] }) =>
    http.post(`${base}/site`, req);
export const tamperRebuild = (alias: string) => http.post<{ files: number }>(`${base}/rebuild`, { alias });
export const tamperScan = (alias: string) => http.post<{ found: number }>(`${base}/scan`, { alias });
export const tamperEvents = (req: { site: string; page: number; pageSize: number }) =>
    http.post<{ items: TamperEvent[]; total: number }>(`${base}/events/search`, req);
export const tamperClearEvents = () => http.post(`${base}/events/clear`, {});
