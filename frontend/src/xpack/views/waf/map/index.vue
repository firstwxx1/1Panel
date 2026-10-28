<template>
    <div :id="chartId" style="height: 420px" />
</template>

<script setup lang="ts">
import { onMounted, onBeforeUnmount, watch } from 'vue';
import echarts from '@/utils/echarts';
import worldJson from './world.json';
import chinaJson from './china.json';
import { countryToWorld, provinceToChina } from './geo';
import i18n from '@/lang';

const props = defineProps<{
    kind: 'world' | 'china';
    data: { name: string; count: number }[];
}>();

const chartId = 'wafMap' + Math.random().toString(36).slice(2);
let chart: echarts.ECharts | null = null;

const render = () => {
    const el = document.getElementById(chartId);
    if (!el) return;
    chart = chart || echarts.init(el);
    const isChina = props.kind === 'china';
    const mapName = isChina ? 'china' : 'world';
    if (!(echarts as any).getMap(mapName)) {
        echarts.registerMap(mapName, isChina ? chinaJson : worldJson);
    }
    const items = (props.data || [])
        .map((d) => ({ name: isChina ? provinceToChina(d.name) : countryToWorld(d.name), value: d.count }))
        .filter((d) => d.name);
    const max = items.reduce((m, x) => Math.max(m, x.value || 0), 0);
    chart.setOption(
        {
            tooltip: {
                trigger: 'item',
                formatter: (p: any) => `${p.name}: ${p.value ?? 0}`,
            },
            // the legend bar under the map is rendered in HTML; keep echarts' own hidden
            visualMap: {
                show: false,
                min: 0,
                max: max || 1,
                inRange: { color: ['#e0f3ff', '#006edd'] },
            },
            series: [
                {
                    type: 'map',
                    map: mapName,
                    emphasis: { label: { show: true } },
                    label: { show: false },
                    itemStyle: { areaColor: 'rgba(0,0,0,0.04)', borderColor: 'rgba(0,0,0,0.15)' },
                    data: items,
                },
            ],
        },
        true,
    );
    chart.resize();
};

watch(() => [props.kind, props.data], render, { deep: true });

let ro: ResizeObserver | undefined;
onMounted(() => {
    render();
    const el = document.getElementById(chartId);
    if (el) {
        ro = new ResizeObserver(() => chart?.resize());
        ro.observe(el);
    }
});
onBeforeUnmount(() => {
    ro?.disconnect();
    chart?.dispose();
});
</script>
