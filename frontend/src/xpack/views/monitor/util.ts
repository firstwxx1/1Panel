import { MonTrend } from '@/xpack/api/monitor';

export const fmtBytes = (n: number) => {
    if (!n || n < 1024) return (n || 0) + ' B';
    const u = ['KB', 'MB', 'GB', 'TB'];
    let i = -1;
    do {
        n /= 1024;
        i++;
    } while (n >= 1024 && i < u.length - 1);
    return n.toFixed(2) + ' ' + u[i];
};

export const fmtNum = (n?: number) => (n ?? 0).toLocaleString();

const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();

export type DatePreset = 'today' | 'yesterday' | '7d' | '30d';

// today..today as a daterange picker default
export const todayRange = (): [Date, Date] => {
    const d = new Date(startOfDay(new Date()));
    return [d, d];
};

export const presetRange = (preset: DatePreset): [Date, Date] => {
    const todayMs = startOfDay(new Date());
    const dayMs = 86400 * 1000;
    if (preset === 'yesterday') {
        const y = new Date(todayMs - dayMs);
        return [y, y];
    }
    if (preset === '7d') {
        return [new Date(todayMs - 6 * dayMs), new Date(todayMs)];
    }
    if (preset === '30d') {
        return [new Date(todayMs - 29 * dayMs), new Date(todayMs)];
    }
    return [new Date(todayMs), new Date(todayMs)];
};

export const detectPreset = (r: Date[] | null): string => {
    if (!r || r.length < 2) return '';
    const s = startOfDay(r[0]);
    const e = startOfDay(r[1]);
    for (const p of ['today', 'yesterday', '7d', '30d'] as DatePreset[]) {
        const [ps, pe] = presetRange(p);
        if (s === ps.getTime() && e === pe.getTime()) return p;
    }
    return '';
};

// a daterange picker value covers whole days: [first day 00:00, last day + 1 day)
export const dayParams = (r: Date[] | null) => {
    if (!r || r.length < 2) return {};
    return { start: Math.floor(startOfDay(r[0]) / 1000), end: Math.floor(startOfDay(r[1]) / 1000) + 86400 };
};

// a datetimerange picker value is used as is
export const timeParams = (r: Date[] | null) => {
    if (!r || r.length < 2) return {};
    return { start: Math.floor(r[0].getTime() / 1000), end: Math.floor(r[1].getTime() / 1000) + 1 };
};

export const trendOption = (trend: MonTrend[], name: string, key: 'pv' | 'uv' | 'req' | 'bytes', color = '#005EEB') => ({
    tooltip: {
        trigger: 'axis',
        formatter: (params: any) => {
            const list = Array.isArray(params) ? params : [params];
            if (!list.length) return '';
            const head = `<div style="margin-bottom:4px;font-weight:600">${list[0].axisValueLabel ?? list[0].name ?? ''}</div>`;
            const lines = list.map((it: any) => {
                const val = key === 'bytes' ? fmtBytes(Number(it.data || 0)) : fmtNum(Number(it.data || 0));
                return `<div>${it.marker} ${it.seriesName}: <b>${val}</b></div>`;
            });
            return head + lines.join('');
        },
    },
    grid: { left: 60, right: 20, top: 24, bottom: 28 },
    legend: { show: false },
    xAxis: { type: 'category', boundaryGap: false, axisLabel: { hideOverlap: true } },
    yAxis: [
        {
            type: 'value',
            minInterval: 1,
            axisLabel: { formatter: key === 'bytes' ? (v: number) => fmtBytes(v) : undefined },
        },
    ],
    xData: trend.map((x) => x.time),
    yData: [
        {
            name,
            data: trend.map((x) => x[key] || 0),
            showSymbol: trend.length <= 31,
            symbolSize: 5,
            itemStyle: { color },
            areaStyle: { color, opacity: 0.12 },
        },
    ],
});
