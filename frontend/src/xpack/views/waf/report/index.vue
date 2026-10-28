<template>
    <div>
        <el-card class="mb-3">
            <el-radio-group v-model="range" @change="load">
                <el-radio-button value="today">{{ $t('pro.waf.today') }}</el-radio-button>
                <el-radio-button value="yesterday">{{ $t('pro.waf.yesterday') }}</el-radio-button>
                <el-radio-button value="7d">{{ $t('pro.waf.last7') }}</el-radio-button>
                <el-radio-button value="30d">{{ $t('pro.waf.last30') }}</el-radio-button>
            </el-radio-group>
            <span class="ml-4">{{ $t('pro.waf.intercept') }}: {{ data.total }}</span>
        </el-card>
        <el-row :gutter="12">
            <el-col :span="12" v-for="b in blocks" :key="b.key">
                <el-card class="mb-3">
                    <template #header>{{ $t('pro.waf.' + b.key) }}</template>
                    <el-table :data="(data as any)[b.field]" size="small">
                        <el-table-column prop="name" show-overflow-tooltip />
                        <el-table-column prop="count" width="100" />
                        <el-table-column width="160">
                            <template #default="{ row }">
                                <el-progress :percentage="percent(row.count)" :show-text="false" />
                            </template>
                        </el-table-column>
                    </el-table>
                </el-card>
            </el-col>
        </el-row>
    </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { getWafReport, WafReport } from '@/xpack/api/waf';

const blocks = [
    { key: 'attackType', field: 'types' },
    { key: 'topIp', field: 'ips' },
    { key: 'topSite', field: 'websites' },
    { key: 'topUrl', field: 'uris' },
    { key: 'topCountry', field: 'countries' },
];
const range = ref('today');
const data = reactive<WafReport>({ total: 0, types: [], ips: [], websites: [], uris: [], countries: [] });

const fmt = (d: Date) => {
    const p = (n: number) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
};

const percent = (count: number) => (data.total ? Math.min(100, Math.round((count / data.total) * 100)) : 0);

const load = async () => {
    const now = new Date();
    const day0 = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    let start = day0;
    let end = now;
    if (range.value === 'yesterday') {
        start = new Date(day0.getTime() - 86400000);
        end = new Date(day0.getTime() - 1000);
    } else if (range.value === '7d') {
        start = new Date(day0.getTime() - 6 * 86400000);
    } else if (range.value === '30d') {
        start = new Date(day0.getTime() - 29 * 86400000);
    }
    const res = await getWafReport({ start: fmt(start), end: fmt(end) });
    Object.assign(data, res.data);
};

onMounted(load);
</script>
