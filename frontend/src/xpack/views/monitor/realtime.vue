<template>
    <div>
        <div class="bar">
            <el-input v-model="ip" clearable :placeholder="$t('pro.monitor.filterIp')" style="width: 180px" @keyup.enter="reload" @clear="reload" />
            <el-input v-model="url" clearable :placeholder="$t('pro.monitor.filterUrl')" style="width: 220px" @keyup.enter="reload" @clear="reload" />
            <el-button :icon="Search" @click="reload" />
            <el-checkbox v-model="auto">{{ $t('pro.monitor.autoRefresh') }}</el-checkbox>
        </div>
        <el-table :data="items" v-loading="loading" size="small">
            <el-table-column :label="$t('pro.monitor.time')" width="170">
                <template #default="{ row }">{{ fmt(row.time) }}</template>
            </el-table-column>
            <el-table-column prop="ip" label="IP" width="140" />
            <el-table-column prop="region" :label="$t('pro.monitor.region')" width="140" show-overflow-tooltip />
            <el-table-column prop="method" :label="$t('pro.monitor.method')" width="80" />
            <el-table-column prop="url" label="URL" min-width="220" show-overflow-tooltip />
            <el-table-column :label="$t('pro.monitor.status')" width="80">
                <template #default="{ row }">
                    <el-tag size="small" :type="row.status >= 500 ? 'danger' : row.status >= 400 ? 'warning' : 'success'" effect="light">
                        {{ row.status }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="$t('pro.monitor.duration')" width="90">
                <template #default="{ row }">{{ row.rt ? (row.rt * 1000).toFixed(0) + ' ms' : '-' }}</template>
            </el-table-column>
            <el-table-column prop="ua" :label="$t('pro.monitor.ua')" min-width="220" show-overflow-tooltip />
            <template #empty>{{ $t('pro.monitor.noData') }}</template>
        </el-table>
        <div class="pager">
            <el-pagination
                v-model:current-page="page"
                v-model:page-size="size"
                :total="total"
                :page-sizes="[20, 50, 100]"
                layout="total, sizes, prev, pager, next"
                @change="load(false)"
            />
        </div>
    </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { Search } from '@element-plus/icons-vue';
import { monRealtime } from '@/xpack/api/monitor';

const props = defineProps<{ site: string }>();
const ip = ref('');
const url = ref('');
const items = ref<any[]>([]);
const total = ref(0);
const page = ref(1);
const size = ref(20);
const loading = ref(false);
const auto = ref(true);
let timer: number | undefined;

const pad = (n: number) => String(n).padStart(2, '0');
const fmt = (ts: number) => {
    const d = new Date(ts * 1000);
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
};
const load = async (silent = false) => {
    if (!silent) loading.value = true;
    try {
        const res = await monRealtime({ site: props.site, ip: ip.value, url: url.value, page: page.value, size: size.value });
        items.value = res.data.items || [];
        total.value = res.data.total;
    } finally {
        loading.value = false;
    }
};
const reload = () => {
    page.value = 1;
    load();
};
const tick = () => {
    if (timer) window.clearInterval(timer);
    timer = auto.value ? window.setInterval(() => page.value === 1 && load(true), 5000) : undefined;
};
watch(auto, tick);
onMounted(() => {
    load();
    tick();
});
onBeforeUnmount(() => timer && window.clearInterval(timer));
</script>

<style scoped>
.bar {
    display: flex;
    gap: 8px;
    align-items: center;
    margin-bottom: 12px;
}
.pager {
    display: flex;
    justify-content: flex-end;
    margin-top: 12px;
}
</style>
