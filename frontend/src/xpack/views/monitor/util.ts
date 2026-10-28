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

const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();

// today..today as a daterange picker default
export const todayRange = (): [Date, Date] => {
    const d = new Date(startOfDay(new Date()));
    return [d, d];
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
    tooltip: { trigger: 'axis' },
    grid: { left: 56, right: 16, top: 24, bottom: 28 },
    legend: { show: false },
    xAxis: { type: 'category', boundaryGap: false, axisLabel: { hideOverlap: true } },
    yAxis: [{ type: 'value', minInterval: 1, axisLabel: { formatter: key === 'bytes' ? (v: number) => fmtBytes(v) : undefined } }],
    xData: trend.map((x) => x.time),
    yData: [{ name, data: trend.map((x) => x[key] || 0), showSymbol: true, symbolSize: 4, itemStyle: { color }, lineStyle: { color } }],
});
