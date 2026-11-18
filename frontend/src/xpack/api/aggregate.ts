import http from '@/api';
import { listAllNodes, nodeQuery as nodeQueryRaw, NodeRow } from '@/xpack/api/node';
import { GlobalStore } from '@/store';

// When the panel sits on the master node ("local"), the self-written WAF / site-monitor
// modules only read the master's own OpenResty. This helper fans read-only calls out to
// every healthy child node and merges them, so the master shows the whole fleet at once.
// Write ops use nodeExec / nodeGetExec so errors are never swallowed.

export interface NodeTagged<T> {
    node: string;
    data: T;
}

const AGG_NODE_TIMEOUT = 12000;

let nodeCache: { at: number; rows: NodeRow[] } | null = null;

export function invalidateNodeCache() {
    nodeCache = null;
}

// readableNodes returns the master plus every bound, healthy child node.
export async function readableNodes(): Promise<NodeRow[]> {
    if (nodeCache && Date.now() - nodeCache.at < 15000) {
        return nodeCache.rows;
    }
    let rows: NodeRow[] = [];
    try {
        const res = await listAllNodes();
        rows = res.data || [];
    } catch {
        rows = [];
    }
    const out = rows.filter((n) => n.isLocal || (n.kind === 'node' && n.isBound !== false && n.status === 'Healthy'));
    nodeCache = { at: Date.now(), rows: out };
    return out;
}

// isMasterView is true when read calls should aggregate across nodes.
export function isMasterView(): boolean {
    return GlobalStore().currentNode === 'local';
}

// Explicit per-node write/config helpers: NEVER set skipErrorMessage or swallow errors.
// Note: api/index.ts request interceptor already calls encodeURIComponent on headers.CurrentNode,
// so pass the raw node name here to avoid double-encoding.
export const nodeExec = <T = any>(node: string, url: string, body: object = {}) =>
    http.postWithConfig<T>(url, body, { headers: { CurrentNode: node || 'local' } });

export const nodeGetExec = <T = any>(node: string, url: string, params: object = {}) =>
    http.get<T>(url, params, { headers: { CurrentNode: node || 'local' } });

// nodeQuery posts `url` to one node with a bounded timeout; failures resolve to undefined
// so one offline/slow node never hangs or breaks the merged master view.
export async function nodeQuery<T>(nodeName: string, url: string, body: object): Promise<T | undefined> {
    try {
        const res = await nodeQueryRaw<T>(nodeName, url, body, AGG_NODE_TIMEOUT);
        return res.data as T;
    } catch {
        return undefined;
    }
}

// nodeGet runs a GET against one node with a bounded timeout and silent error handling.
export async function nodeGet<T>(nodeName: string, url: string, params?: object): Promise<T | undefined> {
    try {
        const res = await http.get<T>(url, params, {
            timeout: AGG_NODE_TIMEOUT,
            headers: { CurrentNode: nodeName },
            skipErrorMessage: true,
        });
        return res.data as T;
    } catch {
        return undefined;
    }
}

// fanOut runs fn against every readable node and tags each successful result with the
// node name, so merged rows can show where they came from.
export async function fanOut<T>(fn: (nodeName: string) => Promise<T | undefined>): Promise<NodeTagged<T>[]> {
    const nodes = await readableNodes();
    const parts = await Promise.all(
        nodes.map(async (n): Promise<NodeTagged<T> | undefined> => {
            const data = await fn(n.name);
            return data === undefined ? undefined : { node: n.name, data };
        }),
    );
    const out: NodeTagged<T>[] = [];
    for (const p of parts) {
        if (p !== undefined) out.push(p);
    }
    return out;
}

// ---------- merge helpers (shared by the waf/monitor aggregation wrappers) ----------

export const zeroTotals = <T extends Record<string, number>>(keys: (keyof T)[]): T => {
    const o: any = {};
    keys.forEach((k) => (o[k] = 0));
    return o as T;
};

// sumFields adds every numeric field across parts; avgRt (if present) is recomputed
// as a request-weighted average afterwards by the caller.
export function sumTotals<T extends Record<string, number>>(parts: T[], keys: (keyof T)[]): T {
    const out: any = zeroTotals(keys);
    for (const p of parts) {
        keys.forEach((k) => (out[k] = (out[k] || 0) + (p[k] || 0)));
    }
    return out as T;
}

// mergeBuckets sums same-named buckets and keeps the top `limit` by count.
export function mergeBuckets<B extends { name: string; count: number }>(lists: B[][], limit = 20): B[] {
    const m = new Map<string, B>();
    for (const list of lists) {
        for (const b of list || []) {
            const cur = m.get(b.name);
            if (cur) {
                cur.count += b.count;
            } else {
                m.set(b.name, { ...(b as any) });
            }
        }
    }
    return [...m.values()].sort((a, b) => b.count - a.count).slice(0, limit);
}

// mergeKeyed merges time-series rows keyed by `key` (day / time bucket), summing all
// numeric fields, and returns them ordered by the key.
export function mergeKeyed<R extends Record<string, any>>(lists: R[][], key: keyof R): R[] {
    const m = new Map<string, R>();
    for (const list of lists) {
        for (const row of list || []) {
            const k = String(row[key]);
            const cur = m.get(k);
            if (!cur) {
                m.set(k, { ...(row as any) });
                continue;
            }
            for (const f of Object.keys(row)) {
                if (f === key) continue;
                if (typeof row[f] === 'number') {
                    (cur as any)[f] = (cur[f] || 0) + row[f];
                }
            }
        }
    }
    return [...m.values()].sort((a, b) => String(a[key]).localeCompare(String(b[key])));
}
