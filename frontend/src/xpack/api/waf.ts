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

// ---------------------------------------------------------------------------
// Aggregated reads: when the panel sits on the master ("local"), these fan the
// read-only queries out to every healthy node and merge the results, so the
// master view shows the whole fleet. Rows carry a `node` field (used for the
// 节点 column and to route per-row write ops back to the node they came from).
// Single-node views are untouched — the plain functions above keep being used.
// ---------------------------------------------------------------------------

import { fanOut, isMasterView, mergeBuckets, mergeKeyed, nodeQuery } from '@/xpack/api/aggregate';

export type WafLogNode = WafAttackLog & { node: string };
export type WafBlockNode = WafBlockItem & { node: string };

export async function getWafOverviewAgg(): Promise<WafOverview> {
    if (!isMasterView()) {
        return (await getWafOverview()).data;
    }
    const parts = await fanOut((n) => nodeQuery<WafOverview>(n, '/xpack/waf/overview', {}));
    const numKeys = ['reqCount', 'attackCount', 'count4xx', 'count5xx'] as const;
    const today: WafDayStat = { day: '', reqCount: 0, attackCount: 0, count4xx: 0, count5xx: 0 };
    for (const p of parts) {
        numKeys.forEach((k) => (today[k] += p.data.today?.[k] || 0));
    }
    today.day = parts.find((p) => p.data.today?.day)?.data.today.day || '';
    const trend = mergeKeyed(
        parts.map((p) => p.data.trend || []),
        'day',
    );
    return { today, trend };
}

export async function searchWafLogsAgg(req: WafLogQuery): Promise<{ total: number; items: WafLogNode[] }> {
    if (!isMasterView()) {
        const res = await searchWafLogs(req);
        return { total: res.data.total, items: (res.data.items || []).map((l) => ({ ...l, node: '' })) };
    }
    const page = req.page || 1;
    const size = req.size || 20;
    // fetch enough rows from every node to cover the requested page after the merge
    const parts = await fanOut((n) =>
        nodeQuery<{ total: number; items: WafAttackLog[] }>(n, '/xpack/waf/logs/search', { ...req, page: 1, size: page * size }),
    );
    const items = parts
        .flatMap((p) => (p.data.items || []).map((l) => ({ ...l, node: p.node })))
        .sort((a, b) => (a.time < b.time ? 1 : -1));
    return { total: parts.reduce((s, p) => s + (p.data.total || 0), 0), items: items.slice((page - 1) * size, page * size) };
}

export async function listWafBlocksAgg(req: WafBlockQuery): Promise<{ total: number; items: WafBlockNode[] }> {
    if (!isMasterView()) {
        const res = await listWafBlocks(req);
        return { total: res.data.total, items: (res.data.items || []).map((b) => ({ ...b, node: '' })) };
    }
    const page = req.page || 1;
    const size = req.size || 20;
    const parts = await fanOut((n) =>
        nodeQuery<{ total: number; items: WafBlockItem[] }>(n, '/xpack/waf/blocks/search', { ...req, page: 1, size: page * size }),
    );
    const items = parts
        .flatMap((p) => (p.data.items || []).map((b) => ({ ...b, node: p.node })))
        .sort((a, b) => (a.blockedAt < b.blockedAt ? 1 : -1));
    return { total: parts.reduce((s, p) => s + (p.data.total || 0), 0), items: items.slice((page - 1) * size, page * size) };
}

export async function getWafReportAgg(req: WafLogQuery): Promise<WafReport> {
    if (!isMasterView()) {
        return (await getWafReport(req)).data;
    }
    const parts = await fanOut((n) => nodeQuery<WafReport>(n, '/xpack/waf/report', req));
    return {
        total: parts.reduce((s, p) => s + (p.data.total || 0), 0),
        types: mergeBuckets(parts.map((p) => p.data.types)),
        ips: mergeBuckets(parts.map((p) => p.data.ips)),
        websites: mergeBuckets(parts.map((p) => p.data.websites)),
        uris: mergeBuckets(parts.map((p) => p.data.uris)),
        countries: mergeBuckets(parts.map((p) => p.data.countries), 100),
        provinces: mergeBuckets(parts.map((p) => p.data.provinces), 100),
    };
}

export async function getWafLogOptionsAgg(): Promise<WafLogOptions> {
    if (!isMasterView()) {
        return (await getWafLogOptions()).data;
    }
    const parts = await fanOut((n) => httpGetNode<WafLogOptions>(n, '/xpack/waf/logs/options'));
    const uniq = (lists: string[][]) => [...new Set(lists.flat())].sort();
    return {
        websites: uniq(parts.map((p) => p.data.websites || [])),
        ruleTypes: uniq(parts.map((p) => p.data.ruleTypes || [])),
        actions: uniq(parts.map((p) => p.data.actions || [])),
    };
}

// GET against one node (the shared nodeQuery helper is POST-only); the master proxies
// it to the child's agent and failures stay silent like the other fan-out calls.
async function httpGetNode<T>(nodeName: string, url: string): Promise<T | undefined> {
    try {
        const res = await http.get<T>(url, undefined, { headers: { CurrentNode: nodeName }, skipErrorMessage: true });
        return res.data as T;
    } catch {
        return undefined;
    }
}
