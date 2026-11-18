import http from '@/api';
import { nodeExec, nodeGetExec } from '@/xpack/api/aggregate';

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

export const tamperGetSettings = (node?: string) =>
    node ? nodeGetExec<TamperSettings>(node, `${base}/settings`) : http.get<TamperSettings>(`${base}/settings`);
export const tamperSaveGlobal = (req: { enabled: boolean; interval: number }, node?: string) =>
    node ? nodeExec(node, `${base}/settings`, req) : http.post(`${base}/settings`, req);
export const tamperSaveSite = (
    req: { alias: string; enabled: boolean; restore: boolean; excludes: string[] },
    node?: string,
) => (node ? nodeExec(node, `${base}/site`, req) : http.post(`${base}/site`, req));
export const tamperRebuild = (alias: string, node?: string) =>
    node ? nodeExec<{ files: number }>(node, `${base}/rebuild`, { alias }) : http.post<{ files: number }>(`${base}/rebuild`, { alias });
export const tamperScan = (alias: string, node?: string) =>
    node ? nodeExec<{ found: number }>(node, `${base}/scan`, { alias }) : http.post<{ found: number }>(`${base}/scan`, { alias });
export const tamperEvents = (req: { site: string; page: number; pageSize: number }, node?: string) =>
    node
        ? nodeExec<{ items: TamperEvent[]; total: number }>(node, `${base}/events/search`, req)
        : http.post<{ items: TamperEvent[]; total: number }>(`${base}/events/search`, req);
export const tamperClearEvents = (node?: string) =>
    node ? nodeExec(node, `${base}/events/clear`, {}) : http.post(`${base}/events/clear`, {});
