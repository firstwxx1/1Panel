<template>
    <div>
        <ProTabBar v-model="active" :tabs="tabItems" @change="onTab" />

        <div v-if="cfg && active === 'channel'">
            <el-card shadow="never">
                <template #header>
                    <div class="card-head">
                        <span class="title">{{ $t('pro.alert.telegram') }}</span>
                        <el-tag size="small" :type="cfg.telegram.enabled ? 'success' : 'info'" effect="light">
                            {{ cfg.telegram.enabled ? $t('pro.alert.enabled') : $t('pro.alert.disabled') }}
                        </el-tag>
                    </div>
                </template>
                <el-form label-width="150px" class="channel-form">
                    <el-form-item :label="$t('pro.alert.enable')">
                        <el-switch v-model="cfg.telegram.enabled" />
                    </el-form-item>
                    <el-form-item :label="$t('pro.alert.token')" :error="tokenErr">
                        <el-input v-model="cfg.telegram.token" show-password placeholder="123456:ABC-DEF..." />
                        <div class="hint">{{ $t('pro.alert.tokenHint') }}</div>
                    </el-form-item>
                    <el-form-item :label="$t('pro.alert.chatId')" :error="chatErr">
                        <div class="input-row">
                            <el-input v-model="cfg.telegram.chatId" />
                            <el-button :loading="fetching" @click="fetchChats">{{ $t('pro.alert.fetchChat') }}</el-button>
                        </div>
                        <div class="hint">{{ $t('pro.alert.chatHint') }}</div>
                    </el-form-item>
                    <el-form-item :label="$t('pro.alert.proxy')">
                        <el-input v-model="cfg.telegram.proxy" placeholder="http://127.0.0.1:7890" />
                        <div class="hint">{{ $t('pro.alert.proxyHint') }}</div>
                    </el-form-item>
                    <el-form-item :label="$t('pro.alert.interval')">
                        <el-input-number v-model="cfg.interval" :min="30" :max="3600" controls-position="right" />
                    </el-form-item>
                    <el-form-item :label="$t('pro.alert.cooldown')">
                        <div class="cooldown-wrap">
                            <el-input-number v-model="cfg.cooldownMin" :min="1" :max="1440" controls-position="right" />
                            <span class="cooldown-hint">{{ $t('pro.alert.cooldownHint') }}</span>
                        </div>
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
                <template #header>
                    <div class="card-head">
                        <span class="title">{{ $t('pro.alert.group_' + g.key) }}</span>
                        <el-tag size="small" :type="groupActiveCount(g) > 0 ? 'primary' : 'info'" effect="plain">
                            {{ $t('pro.alert.activeRules', { on: groupActiveCount(g), total: g.rules.length }) }}
                        </el-tag>
                    </div>
                </template>
                <div class="rule-list">
                    <div v-for="id in g.rules" :key="id" class="rule-row">
                        <div class="rule-left">
                            <el-switch v-model="cfg.rules[id].enabled" />
                            <span class="rule-name" :class="{ 'is-off': !cfg.rules[id].enabled }">
                                {{ $t('pro.alert.r_' + id) }}
                            </span>
                        </div>
                        <div v-if="hasThreshold.includes(id)" class="rule-right">
                            <span class="threshold-label">{{ $t('pro.alert.u_' + id) }}</span>
                            <el-input-number
                                v-model="cfg.rules[id].threshold"
                                :min="0"
                                size="small"
                                controls-position="right"
                                class="threshold-input"
                            />
                        </div>
                    </div>
                </div>
            </el-card>

            <el-card shadow="never" class="save-bar-card">
                <div class="save-bar">
                    <el-button type="primary" :loading="saving" @click="save">{{ $t('pro.alert.save') }}</el-button>
                </div>
            </el-card>
        </div>

        <div v-if="active === 'records'">
            <el-card shadow="never">
                <div class="table-toolbar">
                    <div class="toolbar-left">
                        <el-button
                            type="danger"
                            plain
                            :icon="Delete"
                            :disabled="!records.length"
                            @click="clearRecords"
                        >
                            {{ $t('pro.alert.clear') }}
                        </el-button>
                    </div>
                    <div class="toolbar-right">
                        <el-button :icon="Refresh" circle :loading="loading" @click="loadRecords" />
                    </div>
                </div>

                <el-table :data="records" v-loading="loading">
                    <el-table-column :label="$t('pro.alert.time')" width="180">
                        <template #default="{ row }">{{ new Date(row.time).toLocaleString() }}</template>
                    </el-table-column>
                    <el-table-column :label="$t('pro.alert.rule')" width="170">
                        <template #default="{ row }">
                            <el-tag size="small" type="primary" effect="plain">
                                {{ $t('pro.alert.r_' + row.rule) }}
                            </el-tag>
                        </template>
                    </el-table-column>
                    <el-table-column prop="node" :label="$t('pro.alert.node')" width="140">
                        <template #default="{ row }">
                            <span class="mono">{{ row.node || '-' }}</span>
                        </template>
                    </el-table-column>
                    <el-table-column prop="message" :label="$t('pro.alert.message')" min-width="260" show-overflow-tooltip />
                    <el-table-column :label="$t('pro.alert.sent')" width="150">
                        <template #default="{ row }">
                            <el-tag v-if="row.sent" size="small" type="success" effect="light">
                                {{ $t('pro.alert.sentOk') }}
                            </el-tag>
                            <el-tooltip v-else :content="row.error" :disabled="!row.error">
                                <el-tag size="small" type="danger" effect="light">{{ $t('pro.alert.sentNo') }}</el-tag>
                            </el-tooltip>
                        </template>
                    </el-table-column>
                    <template #empty>
                        <el-empty :image-size="60" :description="$t('commons.msg.noneData')" />
                    </template>
                </el-table>
            </el-card>
        </div>

        <el-dialog v-model="chatDialogVisible" :title="$t('pro.alert.selectChat')" width="460px">
            <div class="hint mb-2">{{ $t('pro.alert.selectChatHint') }}</div>
            <el-select v-model="pickedChatId" style="width: 100%">
                <el-option
                    v-for="c in chatOptions"
                    :key="c.id"
                    :value="c.id"
                    :label="c.name ? `${c.name} (${c.id})` : c.id"
                />
            </el-select>
            <template #footer>
                <el-button @click="chatDialogVisible = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" @click="confirmPickChat">{{ $t('commons.button.confirm') }}</el-button>
            </template>
        </el-dialog>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { Delete, Refresh } from '@element-plus/icons-vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { useI18n } from 'vue-i18n';
import ProTabBar from '@/xpack/components/ProTabBar.vue';
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

const { t } = useI18n();
const keys = ['channel', 'rules', 'records'];
const active = ref('channel');
const tabItems = computed(() =>
    keys.map((k) => ({
        key: k,
        label: t('pro.alert.' + k),
    })),
);

const cfg = ref<AlertConfig | null>(null);
const records = ref<AlertRecord[]>([]);
const loading = ref(false);
const saving = ref(false);
const testing = ref(false);
const fetching = ref(false);
const chatDialogVisible = ref(false);
const chatOptions = ref<{ id: string; name: string }[]>([]);
const pickedChatId = ref('');

const groups = [
    { key: 'site', rules: ['cert_expire', 'site_5xx', 'site_tamper'] },
    { key: 'waf', rules: ['waf_attack'] },
    { key: 'task', rules: ['cron_failed', 'app_error', 'node_offline', 'sync_failed'] },
];
const hasThreshold = ['cert_expire', 'site_5xx', 'waf_attack'];

const groupActiveCount = (g: { rules: string[] }) => {
    if (!cfg.value?.rules) return 0;
    return g.rules.filter((id) => cfg.value?.rules[id]?.enabled).length;
};

const reChatID = /^(-?\d{1,20}|@[A-Za-z][A-Za-z0-9_]{4,31})$/;
const reBotTok = /^\d{5,20}:[A-Za-z0-9_-]{30,}$/;
const chatErr = computed(() => {
    const v = (cfg.value?.telegram.chatId || '').trim();
    if (!v) return '';
    if (reBotTok.test(v)) return t('pro.alert.chatIsToken');
    return reChatID.test(v) ? '' : t('pro.alert.chatInvalid');
});
const tokenErr = computed(() => {
    const v = (cfg.value?.telegram.token || '').trim();
    return !v || v === '********' || reBotTok.test(v) ? '' : t('pro.alert.tokenInvalid');
});
const checkChannel = (forTest: boolean) => {
    const tg = cfg.value!.telegram;
    if ((forTest || tg.enabled) && !(tg.token || '').trim()) {
        ElMessage.error(t('pro.alert.tokenRequired'));
        return false;
    }
    const err = tokenErr.value || chatErr.value;
    if (err) {
        ElMessage.error(err);
        return false;
    }
    if ((forTest || tg.enabled) && !(tg.chatId || '').trim()) {
        ElMessage.error(t('pro.alert.chatRequired'));
        return false;
    }
    return true;
};

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
const onTab = (k: string) => {
    if (k === 'records') loadRecords();
};
const save = async () => {
    if (!checkChannel(false)) return;
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
    if (!checkChannel(true)) return;
    testing.value = true;
    try {
        await testTelegram(cfg.value!.telegram);
        ElMessage.success(t('pro.alert.testOk'));
    } finally {
        testing.value = false;
    }
};
const fetchChats = async () => {
    if (!(cfg.value?.telegram.token || '').trim()) {
        ElMessage.error(t('pro.alert.tokenRequired'));
        return;
    }
    if (tokenErr.value) {
        ElMessage.error(tokenErr.value);
        return;
    }
    fetching.value = true;
    try {
        const list = (await telegramChats(cfg.value!.telegram)).data || [];
        if (!list.length) {
            ElMessage.warning(t('pro.alert.noChat'));
            return;
        }
        if (list.length === 1) {
            cfg.value!.telegram.chatId = list[0].id;
            return;
        }
        chatOptions.value = list;
        const currentId = (cfg.value!.telegram.chatId || '').trim();
        pickedChatId.value = list.some((c) => c.id === currentId) ? currentId : list[list.length - 1].id;
        chatDialogVisible.value = true;
    } finally {
        fetching.value = false;
    }
};
const confirmPickChat = () => {
    if (cfg.value && pickedChatId.value) {
        cfg.value.telegram.chatId = pickedChatId.value;
    }
    chatDialogVisible.value = false;
};
const clearRecords = async () => {
    await ElMessageBox.confirm(t('pro.alert.clear') + '?', { type: 'warning' });
    await clearAlertRecords();
    loadRecords();
};

onMounted(loadCfg);
</script>

<style scoped>
.card-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
}
.title {
    font-weight: 600;
    font-size: 15px;
    color: var(--el-text-color-primary);
}
.channel-form {
    max-width: 720px;
}
.hint {
    margin-top: 4px;
    color: var(--el-text-color-secondary);
    font-size: 12px;
    line-height: 1.5;
    width: 100%;
}
.input-row {
    display: flex;
    gap: 8px;
    width: 100%;
}
.cooldown-wrap {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
}
.cooldown-hint {
    font-size: 12px;
    color: var(--el-text-color-secondary);
}

.rule-list {
    display: flex;
    flex-direction: column;
}
.rule-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 12px 6px;
    border-bottom: 1px solid var(--el-border-color-extra-light);
}
.rule-row:last-child {
    border-bottom: none;
    padding-bottom: 4px;
}
.rule-row:first-child {
    padding-top: 4px;
}
.rule-left {
    display: flex;
    align-items: center;
    gap: 12px;
}
.rule-name {
    font-size: 13.5px;
    font-weight: 500;
    color: var(--el-text-color-primary);
    transition: color 0.15s ease;
}
.rule-name.is-off {
    color: var(--el-text-color-regular);
    font-weight: 400;
}
.rule-right {
    display: flex;
    align-items: center;
    gap: 10px;
}
.threshold-label {
    font-size: 12.5px;
    color: var(--el-text-color-secondary);
    white-space: nowrap;
}
.threshold-input {
    width: 125px;
}

.save-bar-card :deep(.el-card__body) {
    padding: 12px 18px;
}
.save-bar {
    display: flex;
    align-items: center;
    justify-content: flex-start;
}

.table-toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    margin-bottom: 12px;
}
.toolbar-left,
.toolbar-right {
    display: flex;
    align-items: center;
    gap: 8px;
}
.mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
</style>
