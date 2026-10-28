<template>
    <div>
        <el-tabs v-model="active" type="card" class="mb-3" @tab-change="onTab">
            <el-tab-pane v-for="k in keys" :key="k" :label="$t('pro.alert.' + k)" :name="k" />
        </el-tabs>

        <div v-if="cfg && active === 'channel'">
            <el-card shadow="never">
                <div class="title">{{ $t('pro.alert.telegram') }}</div>
                <el-form label-width="140px" class="mt-3" style="max-width: 720px">
                    <el-form-item :label="$t('pro.alert.enable')">
                        <el-switch v-model="cfg.telegram.enabled" />
                    </el-form-item>
                    <el-form-item :label="$t('pro.alert.token')">
                        <el-input v-model="cfg.telegram.token" show-password placeholder="123456:ABC-DEF..." />
                        <div class="hint">{{ $t('pro.alert.tokenHint') }}</div>
                    </el-form-item>
                    <el-form-item :label="$t('pro.alert.chatId')">
                        <div class="row">
                            <el-input v-model="cfg.telegram.chatId" />
                            <el-button :loading="fetching" @click="fetchChats">{{ $t('pro.alert.fetchChat') }}</el-button>
                        </div>
                        <div class="hint">{{ $t('pro.alert.chatHint') }}</div>
                    </el-form-item>
                    <el-form-item :label="$t('pro.alert.proxy')">
                        <el-input v-model="cfg.telegram.proxy" />
                        <div class="hint">{{ $t('pro.alert.proxyHint') }}</div>
                    </el-form-item>
                    <el-form-item :label="$t('pro.alert.interval')">
                        <el-input-number v-model="cfg.interval" :min="30" :max="3600" />
                    </el-form-item>
                    <el-form-item :label="$t('pro.alert.cooldown')">
                        <el-input-number v-model="cfg.cooldownMin" :min="1" :max="1440" />
                        <div class="hint ml-3">{{ $t('pro.alert.cooldownHint') }}</div>
                    </el-form-item>
                    <el-form-item>
                        <el-button type="primary" :loading="saving" @click="save">{{ $t('pro.alert.save') }}</el-button>
                        <el-button :loading="testing" @click="test">{{ $t('pro.alert.test') }}</el-button>
                    </el-form-item>
                </el-form>
            </el-card>
        </div>

        <div v-if="cfg && active === 'rules'">
            <el-card v-for="g in groups" :key="g.key" shadow="never" class="mb-3">
                <div class="title">{{ $t('pro.alert.group_' + g.key) }}</div>
                <div v-for="id in g.rules" :key="id" class="rule">
                    <el-switch v-model="cfg.rules[id].enabled" />
                    <span class="rname">{{ $t('pro.alert.r_' + id) }}</span>
                    <template v-if="hasThreshold.includes(id)">
                        <span class="hint">{{ $t('pro.alert.u_' + id) }}</span>
                        <el-input-number v-model="cfg.rules[id].threshold" :min="0" size="small" />
                    </template>
                </div>
            </el-card>
            <el-button type="primary" :loading="saving" @click="save">{{ $t('pro.alert.save') }}</el-button>
        </div>

        <div v-if="active === 'records'">
            <div class="row mb-3">
                <el-button @click="loadRecords">{{ $t('pro.alert.refresh') }}</el-button>
                <el-button @click="clearRecords">{{ $t('pro.alert.clear') }}</el-button>
            </div>
            <el-table :data="records" v-loading="loading">
                <el-table-column :label="$t('pro.alert.time')" width="180">
                    <template #default="{ row }">{{ new Date(row.time).toLocaleString() }}</template>
                </el-table-column>
                <el-table-column :label="$t('pro.alert.rule')" width="160">
                    <template #default="{ row }">{{ $t('pro.alert.r_' + row.rule) }}</template>
                </el-table-column>
                <el-table-column prop="node" :label="$t('pro.alert.node')" width="140" />
                <el-table-column prop="message" :label="$t('pro.alert.message')" min-width="260" show-overflow-tooltip />
                <el-table-column :label="$t('pro.alert.sent')" width="200">
                    <template #default="{ row }">
                        <el-tag v-if="row.sent" type="success">{{ $t('pro.alert.sentOk') }}</el-tag>
                        <el-tooltip v-else :content="row.error" :disabled="!row.error">
                            <el-tag type="info">{{ $t('pro.alert.sentNo') }}</el-tag>
                        </el-tooltip>
                    </template>
                </el-table-column>
            </el-table>
        </div>
    </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import {
    AlertConfig,
    AlertRecord,
    clearAlertRecords,
    getAlertConfig,
    listAlertRecords,
    saveAlertConfig,
    telegramChats,
    testTelegram,
} from '@/xpack/api/alert';
import i18n from '@/lang';

const t = i18n.global.t;
const keys = ['channel', 'rules', 'records'];
const active = ref('channel');
const cfg = ref<AlertConfig | null>(null);
const records = ref<AlertRecord[]>([]);
const loading = ref(false);
const saving = ref(false);
const testing = ref(false);
const fetching = ref(false);

const groups = [
    { key: 'site', rules: ['cert_expire', 'site_5xx'] },
    { key: 'waf', rules: ['waf_attack'] },
    { key: 'task', rules: ['cron_failed', 'app_error', 'node_offline', 'sync_failed'] },
];
const hasThreshold = ['cert_expire', 'site_5xx', 'waf_attack'];

const loadCfg = async () => {
    const res = await getAlertConfig();
    cfg.value = res.data;
};
const loadRecords = async () => {
    loading.value = true;
    try {
        records.value = (await listAlertRecords()).data || [];
    } finally {
        loading.value = false;
    }
};
const onTab = (k: any) => {
    if (k === 'records') loadRecords();
};
const save = async () => {
    saving.value = true;
    try {
        await saveAlertConfig(cfg.value!);
        ElMessage.success(t('pro.alert.saved'));
        await loadCfg();
    } finally {
        saving.value = false;
    }
};
const test = async () => {
    testing.value = true;
    try {
        await testTelegram(cfg.value!.telegram);
        ElMessage.success(t('pro.alert.testOk'));
    } finally {
        testing.value = false;
    }
};
const fetchChats = async () => {
    fetching.value = true;
    try {
        const list = (await telegramChats(cfg.value!.telegram)).data || [];
        if (!list.length) {
            ElMessage.warning(t('pro.alert.noChat'));
            return;
        }
        cfg.value!.telegram.chatId = list[list.length - 1].id;
    } finally {
        fetching.value = false;
    }
};
const clearRecords = async () => {
    await ElMessageBox.confirm(t('pro.alert.clear') + '?', { type: 'warning' });
    await clearAlertRecords();
    loadRecords();
};

onMounted(loadCfg);
</script>

<style scoped>
.title {
    font-weight: 600;
}
.hint {
    color: var(--el-text-color-secondary);
    font-size: 12px;
    line-height: 1.5;
    width: 100%;
}
.row {
    display: flex;
    gap: 8px;
    width: 100%;
}
.rule {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 12px;
}
.rname {
    min-width: 140px;
}
</style>
