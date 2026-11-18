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
    NodeTagged,
} from '@/xpack/api/aggregate';

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
    rtCount?: number;
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
    orderBy?: 'id' | 'rt' | 'bytes';
    order?: 'asc' | 'desc';
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
    rtCount?: number;
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
    http.post<{ req: number; bytes: number; series: { time: number; req: number; bytes: number }[] }>(`${base}/live`, {
        site,
    });
export const monSites = (req: MonRange) => http.post<MonSiteRow[]>(`${base}/sites`, req);
export const monGetSettings = (node?: string) =>
    node ? nodeGetExec<MonSettings>(node, `${base}/settings`) : http.get<MonSettings>(`${base}/settings`);
export const monSaveSettings = (cfg: Omit<MonSettings, 'siteList' | 'hasRt'>, node?: string) =>
    node ? nodeExec(node, `${base}/settings`, cfg) : http.post(`${base}/settings`, cfg);
export const monEnableRt = (node?: string) =>
    node ? nodeExec(node, `${base}/rt/enable`, {}) : http.post(`${base}/rt/enable`, {});
export const monClear = (site: string, node?: string) =>
    node ? nodeExec(node, `${base}/clear`, { site }) : http.post(`${base}/clear`, { site });

// ---------------------------------------------------------------------------
// Aggregated reads: on the master ("local") these fan read-only queries out
// to every healthy node and merge.
// ---------------------------------------------------------------------------

export type MonHitNode = MonHit & { node: string };
export type MonSiteRowNode = MonSiteRow & { node: string };

export const joinSite = (node: string, alias: string) => `${node}::${alias}`;
export const splitSite = (s: string): { node: string; alias: string } => {
    const i = s.indexOf('::');
    return i < 0 ? { node: '', alias: s } : { node: s.slice(0, i), alias: s.slice(i + 2) };
};

const TOTAL_KEYS: (keyof MonTotals)[] = ['pv', 'uv', 'ip', 'req', 'bytes', 'err4', 'err5', 'avgRt', 'spiders'];

const emptyTotals = (): MonTotals => ({
    pv: 0,
    uv: 0,
    ip: 0,
    req: 0,
    bytes: 0,
    err4: 0,
    err5: 0,
    avgRt: -1,
    spiders: 0,
    rtCount: 0,
});

// sumTotals adds count-like fields; avgRt ignores < 0 sentinels and weights by rtCount.
function sumTotals(parts: MonTotals[]): MonTotals {
    const out = emptyTotals();
    let rtSum = 0;
    let rtReq = 0;
    for (const p of parts) {
        if (!p) continue;
        for (const k of TOTAL_KEYS) {
            if (k === 'avgRt') continue;
            (out as any)[k] += p[k] || 0;
        }
        if (p.avgRt !== undefined && p.avgRt >= 0) {
            const w = p.rtCount ?? (p.avgRt > 0 ? p.req || 0 : 0);
            if (w > 0) {
                rtSum += p.avgRt * w;
                rtReq += w;
            }
        }
    }
    out.rtCount = rtReq;
    out.avgRt = rtReq > 0 ? rtSum / rtReq : -1;
    return out;
}

// routeFans expands a merged site filter ("node::alias") into per-node fan-out calls:
// a specific merged site only queries its own node; '' queries them all.
async function routeFans<T>(
    req: MonRange,
    call: (node: string, alias: string) => Promise<T | undefined>,
): Promise<NodeTagged<T>[]> {
    const { node, alias } = splitSite(req.site || '');
    if (node) {
        const data = await call(node, alias);
        return data === undefined ? [] : [{ node, data }];
    }
    return fanOut((n) => call(n, alias));
}

export async function monOverviewAgg(req: MonRange): Promise<MonOverview> {
    if (!isMasterView()) {
        return (await monOverview(req)).data;
    }
    const parts = await routeFans(req, (n, alias) =>
        nodeQuery<MonOverview>(n, `${base}/overview`, { ...req, site: alias }),
    );
    return {
        current: sumTotals(parts.map((p) => p.data.current)),
        previous: sumTotals(parts.map((p) => p.data.previous)),
        trend: mergeKeyed(
            parts.map((p) => p.data.trend || []),
            'time',
        ),
    };
}

export async function monDistributionAgg(req: MonRange & { dim: string; limit?: number }): Promise<MonDistItem[]> {
    if (!isMasterView()) {
        return (await monDistribution(req)).data;
    }
    const { limit, ...rest } = req;
    const parts = await routeFans(rest, (n, alias) =>
        nodeQuery<MonDistItem[]>(n, `${base}/distribution`, {
            ...rest,
            site: alias,
            dim: req.dim,
            limit: limit ?? 10,
        }),
    );
    return mergeBuckets(
        parts.map((p) => p.data || []),
        limit ?? 10,
    );
}

interface MonLogResp {
    items: MonHit[];
    total: number;
    stats: MonLogStats;
    logSize: number;
}

export async function monLogsAgg(
    req: MonLogReq,
): Promise<{ items: MonHitNode[]; total: number; stats: MonLogStats; logSize: number }> {
    if (!isMasterView()) {
        const res = await monLogs(req);
        return { ...res.data, items: (res.data.items || []).map((h) => ({ ...h, node: '' })) };
    }
    const page = req.page || 1;
    const size = req.size || 20;
    const parts = await routeFans(req, (n, alias) =>
        nodeQuery<MonLogResp>(n, `${base}/realtime`, { ...req, site: alias, page: 1, size: page * size }),
    );
    const dir = req.order === 'asc' ? 1 : -1;
    const items = parts
        .flatMap((p) => (p.data.items || []).map((h) => ({ ...h, node: p.node })))
        .sort((a, b) => {
            if (req.orderBy === 'rt') return ((a.rt || 0) - (b.rt || 0)) * dir;
            if (req.orderBy === 'bytes') return ((a.bytes || 0) - (b.bytes || 0)) * dir;
            return ((a.time || 0) - (b.time || 0)) * dir;
        });
    const statsParts = parts.map((p) => p.data.stats || ({} as MonLogStats));
    const stats: MonLogStats = {
        req: statsParts.reduce((s, x) => s + (x.req || 0), 0),
        ok: statsParts.reduce((s, x) => s + (x.ok || 0), 0),
        err: statsParts.reduce((s, x) => s + (x.err || 0), 0),
        ips: statsParts.reduce((s, x) => s + (x.ips || 0), 0),
        spiders: statsParts.reduce((s, x) => s + (x.spiders || 0), 0),
        bytes: statsParts.reduce((s, x) => s + (x.bytes || 0), 0),
        avgRt: 0,
        rtCount: 0,
    };
    let rtSum = 0;
    let rtReq = 0;
    for (const x of statsParts) {
        if (x && x.avgRt !== undefined && x.avgRt >= 0) {
            const w = x.rtCount ?? (x.avgRt > 0 ? x.req || 0 : 0);
            if (w > 0) {
                rtSum += x.avgRt * w;
                rtReq += w;
            }
        }
    }
    stats.rtCount = rtReq;
    stats.avgRt = rtReq > 0 ? rtSum / rtReq : 0;
    return {
        items: items.slice((page - 1) * size, page * size),
        total: parts.reduce((s, p) => s + (p.data.total || 0), 0),
        stats,
        logSize: parts.reduce((s, p) => s + (p.data.logSize || 0), 0),
    };
}

export const monRealtimeAgg = monLogsAgg;

export async function monErrorsAgg(req: MonErrReq): Promise<{ items: MonErrItem[]; total: number }> {
    if (!isMasterView()) {
        return (await monErrors(req)).data;
    }
    const page = req.page || 1;
    const size = req.size || 20;
    const parts = await routeFans(req, (n, alias) =>
        nodeQuery<{ items: MonErrItem[]; total: number }>(n, `${base}/errors`, {
            ...req,
            site: alias,
            page: 1,
            size: 500,
        }),
    );
    const m = new Map<string, MonErrItem>();
    for (const p of parts) {
        for (const e of p.data.items || []) {
            const k = `${e.status} ${e.url}`;
            const cur = m.get(k);
            if (cur) {
                cur.count += e.count;
                if (e.last > cur.last) cur.last = e.last;
            } else {
                m.set(k, { ...e });
            }
        }
    }
    const items = [...m.values()].sort((a, b) => b.count - a.count);
    return { total: items.length, items: items.slice((page - 1) * size, page * size) };
}

export async function monLiveAgg(
    site: string,
): Promise<{ req: number; bytes: number; series: { time: number; req: number; bytes: number }[] }> {
    if (!isMasterView()) {
        return (await monLive(site)).data;
    }
    const parts = await routeFans({ site }, (n, alias) =>
        nodeQuery<{ req: number; bytes: number; series: { time: number; req: number; bytes: number }[] }>(
            n,
            `${base}/live`,
            { site: alias },
        ),
    );
    const series = mergeKeyed(
        parts.map((p) => p.data.series || []),
        'time',
    ).sort((a, b) => a.time - b.time);
    return {
        req: parts.reduce((s, p) => s + (p.data.req || 0), 0),
        bytes: parts.reduce((s, p) => s + (p.data.bytes || 0), 0),
        series,
    };
}

export async function monSitesAgg(req: MonRange): Promise<MonSiteRowNode[]> {
    if (!isMasterView()) {
        const res = await monSites(req);
        return (res.data || []).map((r) => ({ ...r, node: '' }));
    }
    const parts = await routeFans(req, (n, alias) =>
        nodeQuery<MonSiteRow[]>(n, `${base}/sites`, { ...req, site: alias }),
    );
    return parts.flatMap((p) => (p.data || []).map((r) => ({ ...r, node: p.node })));
}

// mergedSiteList unions every node's siteList for the site selectors; aliases are
// prefixed with the node name so picking one routes later queries back to that node.
export async function mergedSiteList(): Promise<MonSiteItem[]> {
    if (!isMasterView()) {
        return (await monGetSettings()).data.siteList || [];
    }
    const parts = await fanOut((n) => nodeGet<MonSettings>(n, `${base}/settings`));
    const out: MonSiteItem[] = [];
    for (const p of parts) {
        for (const s of p.data.siteList || []) {
            out.push({ ...s, alias: joinSite(p.node, s.alias) });
        }
    }
    return out;
}
