import http from '@/api';
import {
    fanOut,
    isMasterView,
    mergeBuckets,
    mergeKeyed,
    nodeExec,
    nodeGet,
    nodeGetExec,
    nodeQuery,
} from '@/xpack/api/aggregate';

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
    id?: string;
    state?: string;
    name?: string;
    rule?: string;
    type?: string;
    [key: string]: any;
}

export interface GeoRestrictConfig {
    state: 'on' | 'off';
    schemaVersion: number;
    type: 'geoRestrict';
    res: 'geo';
    code: number;
    action: 'deny' | 'allow';
    rules: string[];
    provinceAction?: 'deny' | 'allow';
    provinceRules?: string[];
}

export const getWafOverview = () => http.get<WafOverview>('/xpack/waf/overview');
export const searchWafLogs = (req: WafLogQuery) =>
    http.post<{ total: number; items: WafAttackLog[] }>('/xpack/waf/logs/search', req);
export const listWafBlocks = (req: WafBlockQuery) =>
    http.post<{ total: number; items: WafBlockItem[] }>('/xpack/waf/blocks/search', req);
export const removeWafBlocks = (ips: string[], node?: string) =>
    node ? nodeExec(node, '/xpack/waf/blocks/remove', { ips }) : http.post('/xpack/waf/blocks/remove', { ips });
export const clearWafBlocks = (node?: string) =>
    node ? nodeExec(node, '/xpack/waf/blocks/clear', {}) : http.post('/xpack/waf/blocks/clear');
export const getWafLogOptions = () => http.get<WafLogOptions>('/xpack/waf/logs/options');
export const clearWafLogs = (node?: string) =>
    node ? nodeExec(node, '/xpack/waf/logs/clear', {}) : http.post('/xpack/waf/logs/clear');
export const maintainWafLogs = (node?: string) =>
    node ? nodeExec(node, '/xpack/waf/maintenance', {}) : http.post('/xpack/waf/maintenance', {});
export const exportWafLogs = (req: WafLogQuery) =>
    http.download<BlobPart>('/xpack/waf/logs/export', req, { responseType: 'blob' });
export const exportWafBlocks = (req: WafBlockQuery) =>
    http.download<BlobPart>('/xpack/waf/blocks/export', req, { responseType: 'blob' });
export const getWafReport = (req: WafLogQuery) => http.post<WafReport>('/xpack/waf/report', req);
export const getWafGlobal = (node?: string) =>
    node ? nodeGetExec<Record<string, any>>(node, '/xpack/waf/global') : http.get<Record<string, any>>('/xpack/waf/global');
export const updateWafGlobal = (patch: Record<string, any>, node?: string) =>
    node ? nodeExec(node, '/xpack/waf/global/update', { patch }) : http.post('/xpack/waf/global/update', { patch });
export const listWafSites = (node?: string) =>
    node ? nodeGetExec<WafSite[]>(node, '/xpack/waf/sites') : http.get<WafSite[]>('/xpack/waf/sites');
export const applyWafGlobal = (keys: string[], node?: string) =>
    node ? nodeExec(node, '/xpack/waf/global/apply', { keys }) : http.post('/xpack/waf/global/apply', { keys });
export const updateWafSite = (key: string, patch: Record<string, any>, node?: string) =>
    node ? nodeExec(node, '/xpack/waf/site/update', { key, patch }) : http.post('/xpack/waf/site/update', { key, patch });
export const searchWafRules = (
    name: string,
    site: string,
    search: string,
    page: number,
    size: number,
    node?: string,
) =>
    node
        ? nodeExec<{ total: number; items: WafRule[] }>(node, '/xpack/waf/rules/search', {
              name,
              site,
              search,
              page,
              size,
          })
        : http.post<{ total: number; items: WafRule[] }>('/xpack/waf/rules/search', {
              name,
              site,
              search,
              page,
              size,
          });
export const saveWafRule = (name: string, site: string, rule: WafRule, oldId: string, node?: string) =>
    node
        ? nodeExec(node, '/xpack/waf/rules/save', { name, site, rule, oldId })
        : http.post('/xpack/waf/rules/save', { name, site, rule, oldId });
export const deleteWafRules = (name: string, site: string, ids: string[], node?: string) =>
    node
        ? nodeExec(node, '/xpack/waf/rules/delete', { name, site, ids })
        : http.post('/xpack/waf/rules/delete', { name, site, ids });

// ---------------------------------------------------------------------------
// Aggregated reads: when the panel sits on the master ("local"), these fan the
// read-only queries out to every healthy node and merge the results, so the
// master view shows the whole fleet.
// ---------------------------------------------------------------------------

export type WafLogNode = WafAttackLog & { node: string };
export type WafBlockNode = WafBlockItem & { node: string };

const formatLocalToday = (): string => {
    const now = new Date();
    return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
};

export async function getWafOverviewAgg(): Promise<WafOverview> {
    if (!isMasterView()) {
        return (await getWafOverview()).data;
    }
    const parts = await fanOut((n) => nodeGet<WafOverview>(n, '/xpack/waf/overview'));
    const numKeys = ['reqCount', 'attackCount', 'count4xx', 'count5xx'] as const;
    const localDay = formatLocalToday();
    const today: WafDayStat = { day: localDay, reqCount: 0, attackCount: 0, count4xx: 0, count5xx: 0 };
    for (const p of parts) {
        if (p.data.today?.day === localDay) {
            numKeys.forEach((k) => (today[k] += p.data.today?.[k] || 0));
        }
    }
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
    const parts = await fanOut((n) =>
        nodeQuery<{ total: number; items: WafAttackLog[] }>(n, '/xpack/waf/logs/search', {
            ...req,
            page: 1,
            size: page * size,
        }),
    );
    const items = parts
        .flatMap((p) => (p.data.items || []).map((l) => ({ ...l, node: p.node })))
        .sort((a, b) => (a.time < b.time ? 1 : -1));
    return {
        total: parts.reduce((s, p) => s + (p.data.total || 0), 0),
        items: items.slice((page - 1) * size, page * size),
    };
}

export async function listWafBlocksAgg(req: WafBlockQuery): Promise<{ total: number; items: WafBlockNode[] }> {
    if (!isMasterView()) {
        const res = await listWafBlocks(req);
        return { total: res.data.total, items: (res.data.items || []).map((b) => ({ ...b, node: '' })) };
    }
    const page = req.page || 1;
    const size = req.size || 20;
    const parts = await fanOut((n) =>
        nodeQuery<{ total: number; items: WafBlockItem[] }>(n, '/xpack/waf/blocks/search', {
            ...req,
            page: 1,
            size: page * size,
        }),
    );
    const items = parts
        .flatMap((p) => (p.data.items || []).map((b) => ({ ...b, node: p.node })))
        .sort((a, b) => (a.blockedAt < b.blockedAt ? 1 : -1));
    return {
        total: parts.reduce((s, p) => s + (p.data.total || 0), 0),
        items: items.slice((page - 1) * size, page * size),
    };
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
    const parts = await fanOut((n) => nodeGet<WafLogOptions>(n, '/xpack/waf/logs/options'));
    const uniq = (lists: string[][]) => [...new Set(lists.flat())].sort();
    return {
        websites: uniq(parts.map((p) => p.data.websites || [])),
        ruleTypes: uniq(parts.map((p) => p.data.ruleTypes || [])),
        actions: uniq(parts.map((p) => p.data.actions || [])),
    };
}
