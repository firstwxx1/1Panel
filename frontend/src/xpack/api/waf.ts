import http from '@/api';

export interface WafDayStat {
    day: string;
    reqCount: number;
    attackCount: number;
    count4xx: number;
    count5xx: number;
}

export interface WafOverview {
    today: WafDayStat;
    trend: WafDayStat[];
}

export interface WafLogQuery {
    website?: string;
    ip?: string;
    host?: string;
    url?: string;
    location?: string;
    ruleType?: string;
    action?: string;
    start?: string;
    end?: string;
    page?: number;
    size?: number;
}

export interface WafAttackLog {
    id: number;
    time: string;
    website: string;
    host: string;
    ip: string;
    country: string;
    province: string;
    method: string;
    uri: string;
    rule: string;
    ruleType: string;
    match: string;
    action: string;
    isBlock: boolean;
    status: number;
    userAgent: string;
    request: string;
}

export interface WafBlockQuery {
    ip?: string;
    page?: number;
    size?: number;
}

export interface WafBlockItem {
    id: number;
    ip: string;
    country: string;
    province: string;
    rule: string;
    blockedAt: string;
    blockingTime: number;
    remaining: number;
    status: 'blocking' | 'released';
}

export interface WafLogOptions {
    websites: string[];
    ruleTypes: string[];
    actions: string[];
}

export interface WafBucket {
    name: string;
    count: number;
}

export interface WafReport {
    total: number;
    types: WafBucket[];
    ips: WafBucket[];
    websites: WafBucket[];
    uris: WafBucket[];
    countries: WafBucket[];
    provinces: WafBucket[];
}

export interface WafSite {
    key: string;
    domains: string[];
    remark: string;
    config: Record<string, any>;
}

export interface WafRule {
    state?: string;
    name?: string;
    rule?: string;
    type?: string;
    [key: string]: any;
}

export const getWafOverview = () => http.get<WafOverview>('/xpack/waf/overview');
export const searchWafLogs = (req: WafLogQuery) =>
    http.post<{ total: number; items: WafAttackLog[] }>('/xpack/waf/logs/search', req);
export const listWafBlocks = (req: WafBlockQuery) =>
    http.post<{ total: number; items: WafBlockItem[] }>('/xpack/waf/blocks/search', req);
export const removeWafBlocks = (ips: string[]) => http.post('/xpack/waf/blocks/remove', { ips });
export const clearWafBlocks = () => http.post('/xpack/waf/blocks/clear');
export const getWafLogOptions = () => http.get<WafLogOptions>('/xpack/waf/logs/options');
export const clearWafLogs = () => http.post('/xpack/waf/logs/clear');
export const exportWafLogs = (req: WafLogQuery) =>
    http.download<BlobPart>('/xpack/waf/logs/export', req, { responseType: 'blob' });
export const exportWafBlocks = (req: WafBlockQuery) =>
    http.download<BlobPart>('/xpack/waf/blocks/export', req, { responseType: 'blob' });
export const getWafReport = (req: WafLogQuery) => http.post<WafReport>('/xpack/waf/report', req);
export const getWafGlobal = () => http.get<Record<string, any>>('/xpack/waf/global');
export const updateWafGlobal = (patch: Record<string, any>) => http.post('/xpack/waf/global/update', { patch });
export const listWafSites = () => http.get<WafSite[]>('/xpack/waf/sites');
export const applyWafGlobal = (keys: string[]) => http.post('/xpack/waf/global/apply', { keys });
export const updateWafSite = (key: string, patch: Record<string, any>) =>
    http.post('/xpack/waf/site/update', { key, patch });
export const searchWafRules = (name: string, site: string, search: string, page: number, size: number) =>
    http.post<{ total: number; items: WafRule[] }>('/xpack/waf/rules/search', { name, site, search, page, size });
export const saveWafRule = (name: string, site: string, rule: WafRule, oldId: string) =>
    http.post('/xpack/waf/rules/save', { name, site, rule, oldId });
export const deleteWafRules = (name: string, site: string, ids: string[]) =>
    http.post('/xpack/waf/rules/delete', { name, site, ids });
