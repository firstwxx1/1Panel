<template>
    <div :id="chartId" class="waf-map-canvas" />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import echarts from '@/utils/echarts';
import { GlobalStore } from '@/store';
import worldJson from './world.json';
import chinaJson from './china.json';
import { countryToWorld, provinceToChina } from './geo';

const props = defineProps<{
    kind: 'world' | 'china';
    data: { name: string; count: number }[];
}>();

const globalStore = GlobalStore();
const htmlDark = ref(document.documentElement.classList.contains('dark'));
const isDark = computed(() => globalStore.isDarkTheme || htmlDark.value);

const chartId = 'wafMap' + Math.random().toString(36).slice(2);
let chart: echarts.ECharts | null = null;
let ro: ResizeObserver | undefined;
let themeObserver: MutationObserver | undefined;

const render = () => {
    const el = document.getElementById(chartId);
    if (!el) return;
    chart = chart || echarts.init(el);
    const isChina = props.kind === 'china';
    const mapName = isChina ? 'china' : 'world';
    if (!(echarts as any).getMap(mapName)) {
        echarts.registerMap(mapName, (isChina ? chinaJson : worldJson) as any);
    }
    const items = (props.data || [])
        .map((d) => ({ name: isChina ? provinceToChina(d.name) : countryToWorld(d.name), value: d.count }))
        .filter((d) => d.name);
    const max = items.reduce((m, x) => Math.max(m, x.value || 0), 0);
    const dark = isDark.value;

    chart.setOption(
        {
            tooltip: {
                trigger: 'item',
                backgroundColor: dark ? '#262729' : '#ffffff',
                borderColor: dark ? '#414243' : '#e4e7ed',
                textStyle: { color: dark ? '#e5eaf3' : '#303133', fontSize: 12 },
                formatter: (p: any) => `${p.name}: <b>${Number(p.value || 0).toLocaleString()}</b>`,
            },
            // the legend bar under the map is rendered in HTML; keep echarts' own hidden
            visualMap: {
                show: false,
                min: 0,
                max: max || 1,
                inRange: { color: dark ? ['#1e3a5f', '#3b82f6'] : ['#dbeafe', '#006edd'] },
            },
            series: [
                {
                    type: 'map',
                    map: mapName,
                    emphasis: {
                        label: { show: true, color: dark ? '#ffffff' : '#1f2937' },
                        itemStyle: { areaColor: dark ? '#2563eb' : '#93c5fd' },
                    },
                    label: { show: false },
                    itemStyle: {
                        areaColor: dark ? 'rgba(255, 255, 255, 0.06)' : '#f3f5f9',
                        borderColor: dark ? 'rgba(255, 255, 255, 0.18)' : 'rgba(0, 0, 0, 0.14)',
                    },
                    data: items,
                },
            ],
        },
        true,
    );
    chart.resize();
};

watch(() => [props.kind, props.data, isDark.value], render, { deep: true });

onMounted(() => {
    themeObserver = new MutationObserver(() => {
        htmlDark.value = document.documentElement.classList.contains('dark');
    });
    themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
    render();
    const el = document.getElementById(chartId);
    if (el) {
        ro = new ResizeObserver(() => chart?.resize());
        ro.observe(el);
    }
});

onBeforeUnmount(() => {
    themeObserver?.disconnect();
    ro?.disconnect();
    chart?.dispose();
    chart = null;
});
</script>

<style scoped>
.waf-map-canvas {
    width: 100%;
    height: 420px;
}
</style>
