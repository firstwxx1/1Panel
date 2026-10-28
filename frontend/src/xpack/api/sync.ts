import http from '@/api';

export interface SyncTask {
    id: string;
    kind: 'file' | 'image' | 'cert';
    name: string;
    src: string;
    dsts: string[];
    status: 'running' | 'success' | 'failed';
    progress: number;
    message: string;
    logs: string[];
    createdAt: string;
    endedAt: string;
}

export interface SyncFileItem {
    path: string;
    isDir: boolean;
}

const base = '/core/xpack/sync';

export const syncFiles = (req: { src: string; dst: string; items: SyncFileItem[]; dstDir: string }) =>
    http.post<SyncTask>(`${base}/files`, req);
export const syncImages = (req: { src: string; dsts: string[]; images: string[] }) =>
    http.post<SyncTask>(`${base}/images`, req);
export const syncCerts = (req: { src: string; dsts: string[]; ids: number[] }) =>
    http.post<SyncTask>(`${base}/certs`, req);
export const listSyncTasks = () => http.get<SyncTask[]>(`${base}/tasks`);
export const deleteSyncTasks = (ids: string[]) => http.post(`${base}/tasks/del`, { ids });
