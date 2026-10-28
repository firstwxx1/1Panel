<template>
    <div>
        <el-card>
            <div class="filters">
                <el-select v-model="query.website" clearable :placeholder="$t('pro.waf.website')" style="width: 150px" @change="filter">
                    <el-option v-for="w in options.websites" :key="w" :value="w" :label="w" />
                </el-select>
                <el-select v-model="query.ruleType" clearable :placeholder="$t('pro.waf.hitRule')" style="width: 170px" @change="filter">
                    <el-option v-for="r in options.ruleTypes" :key="r" :value="r" :label="ruleLabel(r)" />
                </el-select>
                <el-select v-model="query.action" clearable :placeholder="$t('pro.waf.action')" style="width: 120px" @change="filter">
                    <el-option :value="''" :label="$t('pro.waf.all')" />
                    <el-option v-for="a in options.actions" :key="a" :value="a" :label="ruleLabel(a)" />
                </el-select>
                <el-input v-model="query.host" clearable :placeholder="$t('pro.waf.hostPlaceholder')" style="width: 170px" />
                <el-input v-model="query.url" clearable :placeholder="$t('pro.waf.urlPlaceholder')" style="width: 190px" />
                <el-input v-model="query.ip" clearable :placeholder="$t('pro.waf.ipPlaceholder')" style="width: 150px" />
                <el-input v-model="query.location" clearable :placeholder="$t('pro.waf.locationPlaceholder')" style="width: 160px" />
                <el-button type="primary" @click="filter">{{ $t('commons.button.search') }}</el-button>
            </div>

            <div class="actions">
                <div class="left">
                    <el-button type="primary" @click="clearAll">{{ $t('pro.waf.clearLogs') }}</el-button>
                    <el-button :disabled="!selected.length" @click="blockSelected">{{ $t('pro.waf.batchBlockIp') }}</el-button>
                    <el-button @click="rangeDialog = true">{{ $t('pro.waf.batchBlockRange') }}</el-button>
                    <el-button :disabled="!selected.length" @click="whiteUrl">{{ $t('pro.waf.batchWhiteUrl') }}</el-button>
                    <el-button type="primary" plain @click="exportCsv">{{ $t('pro.waf.exportLogs') }}</el-button>
                </div>
                <div class="right">
                    <el-button :icon="Refresh" circle @click="search" />
                    <el-select v-model="refresh" style="width: 130px">
                        <el-option :value="0" :label="$t('pro.waf.noRefresh')" />
                        <el-option :value="10" label="10s" />
                        <el-option :value="30" label="30s" />
                        <el-option :value="60" label="60s" />
                    </el-select>
                </div>
            </div>

            <el-table :data="items" v-loading="loading" size="small" @selection-change="(v: WafAttackLog[]) => (selected = v)">
                <el-table-column type="selection" width="45" />
                <el-table-column prop="ip" label="IP" min-width="130" />
                <el-table-column prop="host" :label="$t('pro.waf.domain')" min-width="160" show-overflow-tooltip />
                <el-table-column prop="uri" label="URL" min-width="220" show-overflow-tooltip />
                <el-table-column :label="$t('pro.waf.region')" min-width="120">
                    <template #default="{ row }">{{ [row.country, row.province].filter(Boolean).join(' ') }}</template>
                </el-table-column>
                <el-table-column :label="$t('pro.waf.action')" width="110">
                    <template #default="{ row }">
                        <el-tag size="small" :type="actionType(row.action)">{{ ruleLabel(row.action) }}</el-tag>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('pro.waf.hitRule')" min-width="140">
                    <template #default="{ row }">{{ ruleLabel(row.ruleType) }}</template>
                </el-table-column>
                <el-table-column :label="$t('pro.waf.time')" width="160" prop="time" />
                <el-table-column :label="$t('commons.table.operate')" width="150" fixed="right">
                    <template #default="{ row }">
                        <el-button link type="primary" @click="openDetail(row)">{{ $t('commons.button.view') }}</el-button>
                        <el-button link type="danger" @click="blockOne(row.ip)">{{ $t('pro.waf.blockIp') }}</el-button>
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

        <el-dialog v-model="show" :title="$t('pro.waf.logDetail')" width="720px">
            <el-descriptions :column="2" border size="small" v-if="detail">
                <el-descriptions-item label="IP">{{ detail.ip }}</el-descriptions-item>
                <el-descriptions-item :label="$t('pro.waf.region')">
                    {{ [detail.country, detail.province].filter(Boolean).join(' ') }}
                </el-descriptions-item>
                <el-descriptions-item :label="$t('pro.waf.domain')">{{ detail.host }}</el-descriptions-item>
                <el-descriptions-item :label="$t('pro.waf.method')">{{ detail.method }}</el-descriptions-item>
                <el-descriptions-item label="URL" :span="2">{{ detail.uri }}</el-descriptions-item>
                <el-descriptions-item :label="$t('pro.waf.hitRule')" :span="2">
                    {{ ruleLabel(detail.ruleType) }} / {{ detail.rule }}
                </el-descriptions-item>
                <el-descriptions-item :label="$t('pro.waf.matchValue')" :span="2">{{ detail.match }}</el-descriptions-item>
                <el-descriptions-item label="UA" :span="2">{{ detail.userAgent }}</el-descriptions-item>
            </el-descriptions>
            <pre class="req">{{ detail?.request }}</pre>
        </el-dialog>

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
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { Refresh } from '@element-plus/icons-vue';
import { useI18n } from 'vue-i18n';
import {
    clearWafLogs,
    exportWafLogs,
    getWafLogOptions,
    saveWafRule,
    searchWafLogs,
    WafAttackLog,
    WafLogOptions,
    WafLogQuery,
} from '@/xpack/api/waf';

const { t, te } = useI18n();
const query = reactive<WafLogQuery>({ website: '', ruleType: '', action: '', host: '', url: '', ip: '', location: '', page: 1, size: 20 });
const options = ref<WafLogOptions>({ websites: [], ruleTypes: [], actions: [] });
const items = ref<WafAttackLog[]>([]);
const total = ref(0);
const loading = ref(false);
const selected = ref<WafAttackLog[]>([]);
const detail = ref<WafAttackLog | null>(null);
const rangeDialog = ref(false);
const range = ref('');
const refresh = ref(0);
let timer: number | undefined;

const show = computed({
    get: () => detail.value !== null,
    set: (v) => {
        if (!v) detail.value = null;
    },
});

const ruleLabel = (key: string) => {
    if (!key) return '-';
    const k = 'pro.waf.rule_' + key;
    return te(k) ? t(k) : key;
};

const actionType = (action: string) => {
    if (action === 'deny') return 'danger';
    if (action === 'allow') return 'success';
    if (action === 'observe') return 'warning';
    return 'info';
};

// changing a filter starts from page 1; the pager keeps asking for another page
const filter = () => {
    query.page = 1;
    search();
};

const search = async () => {
    loading.value = true;
    try {
        const res = await searchWafLogs({ ...query });
        items.value = res.data.items;
        total.value = res.data.total;
    } finally {
        loading.value = false;
    }
};

const openDetail = (row: WafAttackLog) => (detail.value = row);

const addBlack = async (rule: string) => {
    await saveWafRule('ipBlack', '', { state: 'on', name: rule, rule, type: 'ipv4' }, '');
    ElMessage.success(t('pro.waf.blockSuccess'));
};

const blockOne = (ip: string) => addBlack(ip);

const blockSelected = async () => {
    for (const row of selected.value) {
        await saveWafRule('ipBlack', '', { state: 'on', name: row.ip, rule: row.ip, type: 'ipv4' }, '');
    }
    ElMessage.success(t('pro.waf.blockSuccess'));
};

const blockRange = async () => {
    if (!range.value) return;
    await addBlack(range.value);
    rangeDialog.value = false;
    range.value = '';
};

const whiteUrl = async () => {
    for (const row of selected.value) {
        await saveWafRule('urlWhite', '', { state: 'on', name: row.uri, rule: row.uri, type: 'url' }, '');
    }
    ElMessage.success(t('pro.waf.whiteSuccess'));
};

const clearAll = async () => {
    await ElMessageBox.confirm(t('pro.waf.clearLogConfirm'), t('pro.waf.clearLogs'), { type: 'warning' });
    await clearWafLogs();
    ElMessage.success(t('pro.waf.clearSuccess'));
    search();
};

const exportCsv = async () => {
    const blob = await exportWafLogs({ ...query });
    const url = window.URL.createObjectURL(new Blob([blob], { type: 'text/csv' }));
    const a = document.createElement('a');
    a.href = url;
    a.download = 'waf-log.csv';
    a.click();
    window.URL.revokeObjectURL(url);
};

watch(refresh, (v) => {
    if (timer) window.clearInterval(timer);
    timer = v > 0 ? window.setInterval(search, v * 1000) : undefined;
});

onMounted(async () => {
    options.value = (await getWafLogOptions()).data || options.value;
    search();
});

onBeforeUnmount(() => {
    if (timer) window.clearInterval(timer);
});
</script>

<style scoped>
.filters {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-bottom: 12px;
}
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
.req {
    margin-top: 12px;
    max-height: 260px;
    overflow: auto;
    white-space: pre-wrap;
    word-break: break-all;
    background: var(--el-fill-color-light);
    padding: 8px;
    font-size: 12px;
}
</style>
