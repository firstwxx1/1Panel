import http from '@/api';

export interface NodeRow {
    id: number;
    kind: 'node' | 'panel';
    name: string;
    addr: string;
    port: number;
    groupID: number;
    groupBelong: string;
    description: string;
    isFavorite: boolean;
    isLocal: boolean;
    status: string;
    message: string;
    version: string;
    systemVersion: string;
    syncStatus: string;
    protocol: string;
    securityEntrance: string;
    hasApiKey: boolean;
    cpuTotal: number;
    cpuUsedPercent: number;
    memoryTotal: number;
    memoryUsedPercent: number;
    load1: number;
}

export interface NodeSearch {
    page: number;
    pageSize: number;
    info?: string;
    groupID?: number;
}

export interface NodeSSH {
    addr: string;
    port: number;
    user: string;
    authMode: string;
    password?: string;
    privateKey?: string;
    passPhrase?: string;
}

export interface NodeCreate {
    name: string;
    groupID?: number;
    description?: string;
    agentPort?: number;
    ssh?: NodeSSH;
    addr?: string;
    port?: number;
    protocol?: string;
    entrance?: string;
    apiKey?: string;
}

export interface NodeUpdate {
    id: number;
    name: string;
    groupID?: number;
    description?: string;
    addr?: string;
    port?: number;
    protocol?: string;
    entrance?: string;
    apiKey?: string;
}

const base = (kind: 'nodes' | 'panels') => `/core/xpack/${kind}`;

export const searchNodes = (kind: 'nodes' | 'panels', req: NodeSearch) =>
    http.post<{ total: number; items: NodeRow[] }>(`${base(kind)}/search`, req);
export const createNode = (kind: 'nodes' | 'panels', req: NodeCreate) =>
    http.post<{ id: number; token: string }>(`${base(kind)}/create`, req);
export const updateNode = (kind: 'nodes' | 'panels', req: NodeUpdate) => http.post(`${base(kind)}/update`, req);
export const deleteNodes = (kind: 'nodes' | 'panels', ids: number[]) => http.post(`${base(kind)}/del`, { ids });
export const changeNodeGroup = (kind: 'nodes' | 'panels', ids: number[], groupID: number) =>
    http.post(`${base(kind)}/group`, { ids, groupID });
export const testNode = (kind: 'nodes' | 'panels', id: number) => http.post(`${base(kind)}/test`, { id });
export const restartNode = (id: number) => http.post(`${base('nodes')}/restart`, { id });
export const nodeJoinCommand = (token: string) => http.post<string>(`${base('nodes')}/command`, { token });
export const listAllNodes = () => http.post<NodeRow[]>('/core/nodes/list', { type: 'all' });

// Reads another node's own resources through the master; failures stay silent so one
// offline node never floods the page with toasts.
export const nodeQuery = <T = any>(node: string, url: string, body: object) =>
    http.postWithConfig<T>(url, body, { headers: { CurrentNode: node }, skipErrorMessage: true });
