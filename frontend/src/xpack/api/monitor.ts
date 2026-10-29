import http from '@/api';

export interface MonTotals {
    pv: number;
    uv: number;
    ip: number;
    req: number;
    bytes: number;
    err4: number;
    err5: number;
    avgRt: number;
    spiders: number;
}
export interface MonTrend {
    time: string;
    pv: number;
    uv: number;
    req: number;
    bytes: number;
    err4: number;
    err5: number;
}
export interface MonOverview {
    current: MonTotals;
    previous: MonTotals;
    trend: MonTrend[];
}
export interface MonDistItem {
    name: string;
    count: number;
}
export interface MonHit {
    time: number;
    site: string;
    ip: string;
    region: string;
    method: string;
    url: string;
    status: number;
    bytes: number;
    rt: number;
    ua: string;
    referer: string;
}
export interface MonErrItem {
    status: number;
    url: string;
    count: number;
    last: number;
}
export interface MonErrReq extends MonRange {
    class?: string;
    page: number;
    size: number;
}
export interface MonSiteItem {
    alias: string;
    domain: string;
    enabled: boolean;
}
export interface MonRules {
    excludeStatus: string[];
    excludeSuffix: string[];
    excludeUri: string[];
    excludeIps: string[];
    excludeUa: string[];
    cdn: boolean;
}
export interface MonSettings {
    enabled: boolean;
    sites: Record<string, boolean>;
    retentionDays: number;
    detailMax: number;
    excludeIps: string[];
    excludeSuffix: string[];
    staticSuffix: string[];
    excludeStatus: string[];
    excludeUri: string[];
    excludeUa: string[];
    cdn: boolean;
    siteRules: Record<string, MonRules>;
    siteList: MonSiteItem[];
    hasRt: boolean;
}
export interface MonRange {
    site: string;
    range?: string;
    start?: number;
    end?: number;
}
export interface MonLogReq extends MonRange {
    ip?: string;
    url?: string;
    region?: string;
    method?: string;
    status?: string;
    spider?: string;
    page: number;
    size: number;
}
export interface MonLogStats {
    req: number;
    ok: number;
    err: number;
    ips: number;
    spiders: number;
    avgRt: number;
    bytes: number;
}
export interface MonSiteRow extends MonTotals {
    alias: string;
    domain: string;
    enabled: boolean;
}

const base = '/xpack/monitor';
export const monOverview = (req: MonRange) => http.post<MonOverview>(`${base}/overview`, req);
export const monDistribution = (req: MonRange & { dim: string; limit?: number }) =>
    http.post<MonDistItem[]>(`${base}/distribution`, { limit: 10, ...req });
export const monLogs = (req: MonLogReq) =>
    http.post<{ items: MonHit[]; total: number; stats: MonLogStats; logSize: number }>(`${base}/realtime`, req);
export const monRealtime = (req: MonLogReq) =>
    http.post<{ items: MonHit[]; total: number; stats: MonLogStats; logSize: number }>(`${base}/realtime`, req);
export const monErrors = (req: MonErrReq) => http.post<{ items: MonErrItem[]; total: number }>(`${base}/errors`, req);
export const monLive = (site: string) =>
    http.post<{ req: number; bytes: number; series: { time: number; req: number; bytes: number }[] }>(`${base}/live`, { site });
export const monSites = (req: MonRange) => http.post<MonSiteRow[]>(`${base}/sites`, req);
export const monGetSettings = () => http.get<MonSettings>(`${base}/settings`);
export const monSaveSettings = (cfg: Omit<MonSettings, 'siteList' | 'hasRt'>) => http.post(`${base}/settings`, cfg);
export const monEnableRt = () => http.post(`${base}/rt/enable`, {});
export const monClear = (site: string) => http.post(`${base}/clear`, { site });
