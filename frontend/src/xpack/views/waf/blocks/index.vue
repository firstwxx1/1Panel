<template>
    <div>
        <el-card>
            <div class="actions">
                <div class="left">
                    <el-button @click="clearAll">{{ $t('pro.waf.clearLogs') }}</el-button>
                    <el-button :disabled="!selected.length" @click="blockSelected">{{ $t('pro.waf.batchBlockIp') }}</el-button>
                    <el-button @click="rangeDialog = true">{{ $t('pro.waf.batchBlockRange') }}</el-button>
                    <el-button plain type="primary" @click="exportCsv">{{ $t('pro.waf.exportLogs') }}</el-button>
                </div>
                <div class="right">
                    <el-input v-model="query.ip" clearable :placeholder="$t('pro.waf.ipPlaceholder')" style="width: 170px" @change="search">
                        <template #prefix><el-icon><Search /></el-icon></template>
                    </el-input>
                    <el-button :icon="Refresh" circle @click="search" />
                    <el-select v-model="refresh" style="width: 130px">
                        <el-option :value="0" :label="$t('pro.waf.noRefresh')" />
                        <el-option :value="10" label="10s" />
                        <el-option :value="30" label="30s" />
                        <el-option :value="60" label="60s" />
                    </el-select>
                </div>
            </div>

            <el-alert :title="$t('pro.waf.blockWarning')" type="warning" :closable="false" class="mb-3" />

            <el-table :data="items" v-loading="loading" size="small" @selection-change="(v: WafBlockItem[]) => (selected = v)">
                <el-table-column type="selection" width="45" />
                <el-table-column prop="ip" label="IP" min-width="140" />
                <el-table-column prop="blockedAt" :label="$t('pro.waf.time')" width="170" />
                <el-table-column :label="$t('pro.waf.status')" width="110">
                    <template #default="{ row }">
                        <el-tag size="small" :type="row.status === 'blocking' ? 'danger' : 'info'">
                            {{ row.status === 'blocking' ? $t('pro.waf.blocking') : $t('pro.waf.released') }}
                        </el-tag>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('pro.waf.blockDuration')" width="120">
                    <template #default="{ row }">{{ humanTime(row.blockingTime) }}</template>
                </el-table-column>
                <el-table-column :label="$t('pro.waf.region')" min-width="120">
                    <template #default="{ row }">{{ [row.country, row.province].filter(Boolean).join(' ') }}</template>
                </el-table-column>
                <el-table-column :label="$t('pro.waf.hitRule')" min-width="120">
                    <template #default="{ row }">{{ ruleLabel(row.rule) }}</template>
                </el-table-column>
                <el-table-column :label="$t('commons.table.operate')" width="150" fixed="right">
                    <template #default="{ row }">
                        <el-button v-if="row.status === 'blocking'" link type="primary" @click="unblock([row.ip])">
                            {{ $t('pro.waf.unblock') }}
                        </el-button>
                        <el-button link type="danger" @click="addBlack(row.ip)">{{ $t('pro.waf.blockIp') }}</el-button>
                    </template>
                </el-table-column>
            </el-table>
            <div class="pager">
                <el-pagination
                    v-model:current-page="query.page"
                    v-model:page-size="query.size"
                    :total="total"
                    :page-sizes="[20, 50, 100]"
                    layout="total, sizes, prev, pager, next, jumper"
                    small
                    @change="search"
                />
            </div>
        </el-card>

        <el-dialog v-model="rangeDialog" :title="$t('pro.waf.batchBlockRange')" width="480px">
            <el-form label-position="top">
                <el-form-item :label="$t('pro.waf.ipRange')">
                    <el-input v-model="range" :placeholder="$t('pro.waf.rangePlaceholder')" />
                </el-form-item>
            </el-form>
            <template #footer>
                <el-button @click="rangeDialog = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" @click="blockRange">{{ $t('commons.button.confirm') }}</el-button>
            </template>
        </el-dialog>
    </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { Refresh, Search } from '@element-plus/icons-vue';
import { useI18n } from 'vue-i18n';
import {
    clearWafBlocks,
    exportWafBlocks,
    listWafBlocks,
    removeWafBlocks,
    saveWafRule,
    WafBlockItem,
    WafBlockQuery,
} from '@/xpack/api/waf';

const { t, te } = useI18n();
const query = reactive<WafBlockQuery>({ ip: '', page: 1, size: 20 });
const items = ref<WafBlockItem[]>([]);
const total = ref(0);
const loading = ref(false);
const selected = ref<WafBlockItem[]>([]);
const rangeDialog = ref(false);
const range = ref('');
const refresh = ref(0);
let timer: number | undefined;

const ruleLabel = (key: string) => {
    if (!key) return '-';
    const k = 'pro.waf.rule_' + key;
    return te(k) ? t(k) : key;
};

const humanTime = (sec: number) => {
    if (!sec) return '-';
    if (sec % 3600 === 0) return `${sec / 3600}${t('pro.waf.hour')}`;
    if (sec % 60 === 0) return `${sec / 60}${t('pro.waf.minute')}`;
    return `${sec}${t('pro.waf.second')}`;
};

const search = async () => {
    loading.value = true;
    try {
        const res = await listWafBlocks({ ...query });
        items.value = res.data.items;
        total.value = res.data.total;
    } finally {
        loading.value = false;
    }
};

const unblock = async (ips: string[]) => {
    await removeWafBlocks(ips);
    ElMessage.success(t('pro.waf.unblockSuccess'));
    search();
};

const blockSelected = async () => {
    for (const row of selected.value) {
        await saveWafRule('ipBlack', '', { state: 'on', name: row.ip, rule: row.ip, type: 'ipv4' }, '');
    }
    ElMessage.success(t('pro.waf.blockSuccess'));
};

const addBlack = async (ip: string) => {
    await saveWafRule('ipBlack', '', { state: 'on', name: ip, rule: ip, type: 'ipv4' }, '');
    ElMessage.success(t('pro.waf.blockSuccess'));
};

const blockRange = async () => {
    if (!range.value) return;
    await saveWafRule('ipBlack', '', { state: 'on', name: range.value, rule: range.value, type: 'ipArr' }, '');
    ElMessage.success(t('pro.waf.blockSuccess'));
    rangeDialog.value = false;
    range.value = '';
};

const clearAll = async () => {
    await ElMessageBox.confirm(t('pro.waf.clearBlockConfirm'), t('pro.waf.clearLogs'), { type: 'warning' });
    await clearWafBlocks();
    ElMessage.success(t('pro.waf.clearSuccess'));
    search();
};

const exportCsv = async () => {
    const blob = await exportWafBlocks({ ...query });
    const url = window.URL.createObjectURL(new Blob([blob], { type: 'text/csv' }));
    const a = document.createElement('a');
    a.href = url;
    a.download = 'waf-block.csv';
    a.click();
    window.URL.revokeObjectURL(url);
};

watch(refresh, (v) => {
    if (timer) window.clearInterval(timer);
    timer = v > 0 ? window.setInterval(search, v * 1000) : undefined;
});

onMounted(search);

onBeforeUnmount(() => {
    if (timer) window.clearInterval(timer);
});
</script>

<style scoped>
.actions {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 12px;
}
.right {
    display: flex;
    align-items: center;
    gap: 8px;
}
.pager {
    display: flex;
    justify-content: flex-end;
    margin-top: 12px;
}
</style>
