<template>
    <div>
        <el-card shadow="never" class="mb-3">
            <div class="toolbar">
                <div class="toolbar-left">
                    <SiteSelect v-model="site" :sites="sites" all @update:model-value="load" />
                    <el-radio-group v-model="preset" @change="onPreset">
                        <el-radio-button value="today">{{ $t('pro.monitor.today') }}</el-radio-button>
                        <el-radio-button value="yesterday">{{ $t('pro.monitor.yesterday') }}</el-radio-button>
                        <el-radio-button value="7d">{{ $t('pro.monitor.d7') }}</el-radio-button>
                        <el-radio-button value="30d">{{ $t('pro.monitor.d30') }}</el-radio-button>
                    </el-radio-group>
                    <el-date-picker
                        v-model="dates"
                        type="daterange"
                        class="date-picker"
                        :clearable="false"
                        :start-placeholder="$t('pro.monitor.startDate')"
                        :end-placeholder="$t('pro.monitor.endDate')"
                        @change="onDateChange"
                    />
                </div>
                <div class="toolbar-right">
                    <el-button :icon="Refresh" circle :loading="loading" @click="load" />
                </div>
            </div>
        </el-card>

        <div class="grid-row-3 mb-3" v-loading="loading">
            <el-card v-for="d in first" :key="d.dim" shadow="never" class="dist-card">
                <template #header>
                    <div class="head">
                        <span class="card-title">{{ $t('pro.monitor.' + d.key) }}</span>
                        <el-link v-if="d.more" type="primary" :underline="false" @click="more(d)">
                            {{ $t('pro.monitor.more') }}
                        </el-link>
                    </div>
                </template>
                <DistList :items="d.items" :mono="d.mono" />
            </el-card>
        </div>

        <div class="grid-row-4" v-loading="loading">
            <el-card v-for="d in second" :key="d.dim" shadow="never" class="dist-card">
                <template #header>
                    <div class="head">
                        <span class="card-title">{{ $t('pro.monitor.' + d.key) }}</span>
                        <el-link v-if="d.more" type="primary" :underline="false" @click="more(d)">
                            {{ $t('pro.monitor.more') }}
                        </el-link>
                    </div>
                </template>
                <DistList :items="d.items" :mono="d.mono" />
            </el-card>
        </div>

        <el-dialog v-model="dialog" :title="dialogTitle" width="620px">
            <div class="dialog-scroll">
                <DistList :items="dialogItems" :mono="dialogMono" />
            </div>
        </el-dialog>
    </div>
</template>

<script setup lang="ts">
import { defineComponent, h, onMounted, ref } from 'vue';
import { Refresh } from '@element-plus/icons-vue';
import { ElEmpty } from 'element-plus';
import { useI18n } from 'vue-i18n';
import SiteSelect from './SiteSelect.vue';
import { monDistributionAgg, MonDistItem, MonSiteItem } from '@/xpack/api/monitor';
import { DatePreset, dayParams, detectPreset, fmtNum, presetRange, todayRange } from './util';

defineProps<{ sites: MonSiteItem[] }>();
const { t } = useI18n();
const site = ref('');
const preset = ref<string>('today');
const dates = ref<Date[]>(todayRange());
const loading = ref(false);

interface Block {
    dim: string;
    key: string;
    more: boolean;
    mono?: boolean;
    items: MonDistItem[];
}

const first = ref<Block[]>([
    { dim: 'url', key: 'url', more: true, mono: true, items: [] },
    { dim: 'referer', key: 'source', more: true, items: [] },
    { dim: 'ip', key: 'ipTop', more: true, mono: true, items: [] },
]);
const second = ref<Block[]>([
    { dim: 'browser', key: 'browser', more: true, items: [] },
    { dim: 'os', key: 'os', more: false, items: [] },
    { dim: 'device', key: 'device', more: false, items: [] },
    { dim: 'status', key: 'status', more: false, mono: true, items: [] },
]);

const DistList = defineComponent({
    props: {
        items: { type: Array as () => MonDistItem[], required: true },
        mono: { type: Boolean, default: false },
    },
    setup(p) {
        return () => {
            if (!p.items.length) {
                return h('div', { class: 'dist-empty' }, [
                    h(ElEmpty, { imageSize: 48, description: t('pro.monitor.noData') }),
                ]);
            }
            const sum = p.items.reduce((acc, x) => acc + (x.count || 0), 0) || 1;
            return h(
                'div',
                { class: 'dist-list' },
                p.items.map((it, idx) => {
                    const pct = Math.min(100, Math.max(0, (it.count / sum) * 100));
                    return h('div', { class: 'dist-item', key: `${idx}-${it.name}` }, [
                        h('div', { class: 'dist-top' }, [
                            h('div', { class: 'dist-left' }, [
                                h('span', { class: ['rank-badge', idx < 3 ? 'is-top' : ''] }, String(idx + 1)),
                                h(
                                    'span',
                                    { class: ['dist-name', p.mono ? 'mono' : ''], title: it.name },
                                    it.name || '-',
                                ),
                            ]),
                            h('div', { class: 'dist-meta' }, [
                                h('span', { class: 'dist-count' }, fmtNum(it.count)),
                                h('span', { class: 'dist-pct' }, `${pct.toFixed(1)}%`),
                            ]),
                        ]),
                        h('div', { class: 'dist-track' }, [
                            h('div', {
                                class: 'dist-bar',
                                style: { width: `${it.count > 0 ? Math.max(pct, 2) : 0}%` },
                            }),
                        ]),
                    ]);
                }),
            );
        };
    },
});

const onPreset = (val: string | number | boolean | undefined) => {
    if (!val) return;
    dates.value = presetRange(val as DatePreset);
    load();
};
const onDateChange = () => {
    preset.value = detectPreset(dates.value);
    load();
};

const query = (dim: string, limit: number) =>
    monDistributionAgg({ site: site.value, dim, limit, ...dayParams(dates.value) });

const load = async () => {
    loading.value = true;
    try {
        const all = [...first.value, ...second.value];
        const res = await Promise.all(all.map((b) => query(b.dim, 10)));
        all.forEach((b, i) => (b.items = res[i] || []));
    } finally {
        loading.value = false;
    }
};

const dialog = ref(false);
const dialogTitle = ref('');
const dialogMono = ref(false);
const dialogItems = ref<MonDistItem[]>([]);
const more = async (b: Block) => {
    dialogTitle.value = t('pro.monitor.' + b.key);
    dialogMono.value = !!b.mono;
    dialogItems.value = (await query(b.dim, 100)) || [];
    dialog.value = true;
};

onMounted(load);
</script>

<style scoped>
.toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
}
.toolbar-left {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
}
.toolbar-right {
    display: flex;
    align-items: center;
    gap: 8px;
}
.date-picker {
    width: 280px;
    flex: none;
}

.grid-row-3 {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 12px;
    align-items: stretch;
}
.grid-row-4 {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 12px;
    align-items: stretch;
}

.dist-card {
    height: 100%;
    min-height: 320px;
    display: flex;
    flex-direction: column;
}
.dist-card :deep(.el-card__body) {
    flex: 1;
    display: flex;
    flex-direction: column;
}
.head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
}
.card-title {
    font-weight: 600;
    font-size: 14px;
    color: var(--el-text-color-primary);
}

:deep(.dist-empty) {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
}
:deep(.dist-list) {
    display: flex;
    flex-direction: column;
    gap: 10px;
}
:deep(.dist-item) {
    display: flex;
    flex-direction: column;
    gap: 5px;
}
:deep(.dist-top) {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    font-size: 13px;
}
:deep(.dist-left) {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    flex: 1;
}
:deep(.rank-badge) {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 20px;
    height: 20px;
    padding: 0 4px;
    flex-shrink: 0;
    border-radius: 4px;
    font-size: 11px;
    font-weight: 600;
    color: var(--el-text-color-secondary);
    background: var(--el-fill-color);
}
:deep(.rank-badge.is-top) {
    color: var(--el-color-primary);
    background: var(--el-color-primary-light-9);
}
:deep(.dist-name) {
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
    color: var(--el-text-color-regular);
}
:deep(.dist-name.mono) {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 12.5px;
}
:deep(.dist-meta) {
    display: inline-flex;
    align-items: baseline;
    gap: 6px;
    flex-shrink: 0;
    font-variant-numeric: tabular-nums;
}
:deep(.dist-count) {
    font-weight: 600;
    color: var(--el-text-color-primary);
}
:deep(.dist-pct) {
    font-size: 11.5px;
    color: var(--el-text-color-secondary);
    min-width: 42px;
    text-align: right;
}
:deep(.dist-track) {
    height: 4px;
    width: 100%;
    border-radius: 2px;
    background: var(--el-fill-color-light);
    overflow: hidden;
}
:deep(.dist-bar) {
    height: 100%;
    border-radius: 2px;
    background: var(--el-color-primary);
    transition: width 0.25s ease;
}

.dialog-scroll {
    max-height: 60vh;
    overflow-y: auto;
    padding-right: 6px;
}

@media (max-width: 1359px) {
    .grid-row-4 {
        grid-template-columns: repeat(2, minmax(0, 1fr));
    }
}
@media (max-width: 1100px) {
    .grid-row-3 {
        grid-template-columns: repeat(2, minmax(0, 1fr));
    }
}
@media (max-width: 720px) {
    .grid-row-3,
    .grid-row-4 {
        grid-template-columns: minmax(0, 1fr);
    }
    .date-picker {
        width: 100%;
    }
}
</style>
