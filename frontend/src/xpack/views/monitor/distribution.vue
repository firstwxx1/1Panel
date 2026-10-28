<template>
    <div v-loading="loading">
        <el-card shadow="never" class="mb-3">
            <template #header>
                <div class="head">
                    <span class="card-title">{{ $t('pro.monitor.regionMap') }}</span>
                    <el-radio-group v-model="mapKind" size="small">
                        <el-radio-button value="world">{{ $t('pro.monitor.world') }}</el-radio-button>
                        <el-radio-button value="china">{{ $t('pro.monitor.china') }}</el-radio-button>
                    </el-radio-group>
                </div>
            </template>
            <div class="map-wrap">
                <div class="map">
                    <WafMap :kind="mapKind" :data="mapData" />
                </div>
                <div class="side">
                    <div class="side-row side-head">
                        <span>{{ $t('pro.monitor.region') }}</span>
                        <span>{{ $t('pro.monitor.count') }}</span>
                    </div>
                    <div v-for="c in mapData.slice(0, 12)" :key="c.name" class="side-row">
                        <span class="ellipsis" :title="c.name">{{ c.name }}</span>
                        <span>{{ c.count }}</span>
                    </div>
                    <el-empty v-if="!mapData.length" :image-size="48" :description="$t('pro.monitor.noData')" />
                </div>
            </div>
        </el-card>
        <el-row :gutter="12">
            <el-col v-for="d in dims" :key="d.dim" :xs="24" :md="12" :lg="8" class="mb-3">
                <el-card shadow="never" class="list">
                    <template #header>
                        <span class="card-title">{{ d.title }}</span>
                    </template>
                    <div class="side-row side-head">
                        <span>{{ d.title }}</span>
                        <span>{{ $t('pro.monitor.count') }}</span>
                    </div>
                    <div v-for="it in d.items" :key="it.name" class="side-row">
                        <span class="ellipsis" :title="it.name">{{ it.name }}</span>
                        <span>{{ it.count }}</span>
                    </div>
                    <el-empty v-if="!d.items.length" :image-size="40" :description="$t('pro.monitor.noData')" />
                </el-card>
            </el-col>
        </el-row>
    </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue';
import WafMap from '@/xpack/views/waf/map/index.vue';
import { monDistribution, MonDistItem } from '@/xpack/api/monitor';
import i18n from '@/lang';

const props = defineProps<{ site: string; range: string }>();
const t = i18n.global.t;
const loading = ref(false);
const mapKind = ref<'world' | 'china'>('world');
const countries = ref<MonDistItem[]>([]);
const provinces = ref<MonDistItem[]>([]);
const mapData = ref<MonDistItem[]>([]);
const dims = ref<{ dim: string; title: string; items: MonDistItem[] }[]>([]);

const spec = [
    { dim: 'status', key: 'status' },
    { dim: 'referer', key: 'referer' },
    { dim: 'url', key: 'url' },
    { dim: 'ip', key: 'topIp' },
    { dim: 'browser', key: 'browser' },
    { dim: 'os', key: 'os' },
    { dim: 'device', key: 'device' },
    { dim: 'spider', key: 'spider' },
];

const pick = () => {
    mapData.value = mapKind.value === 'china' ? provinces.value : countries.value;
};
watch(mapKind, pick);

onMounted(async () => {
    loading.value = true;
    try {
        const [c, p, ...rest] = await Promise.all([
            monDistribution(props.site, props.range, 'country', 100),
            monDistribution(props.site, props.range, 'province', 100),
            ...spec.map((s) => monDistribution(props.site, props.range, s.dim, 10)),
        ]);
        countries.value = c.data || [];
        provinces.value = p.data || [];
        pick();
        dims.value = spec.map((s, i) => ({ dim: s.dim, title: t('pro.monitor.' + s.key), items: rest[i].data || [] }));
    } finally {
        loading.value = false;
    }
});
</script>

<style scoped>
.head {
    display: flex;
    justify-content: space-between;
    align-items: center;
}
.map-wrap {
    display: flex;
    gap: 16px;
}
.map {
    flex: 1;
    min-width: 0;
}
.side {
    width: 240px;
}
.side-row {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    padding: 5px 0;
    font-size: 13px;
    border-bottom: 1px solid var(--el-border-color-extra-light);
}
.side-head {
    color: var(--el-text-color-secondary);
}
.ellipsis {
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
}
.list {
    min-height: 260px;
}
</style>
