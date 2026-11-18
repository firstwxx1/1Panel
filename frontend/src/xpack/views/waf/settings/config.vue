<template>
    <div class="waf-config">
        <el-card shadow="never" class="mb-3">
            <el-form label-width="180px" style="max-width: 680px">
                <el-form-item :label="$t('pro.waf.logEnable')">
                    <el-switch v-model="form.log.state" active-value="on" inactive-value="off" />
                </el-form-item>
                <el-form-item :label="$t('pro.waf.logRetention')">
                    <el-input-number v-model="form.log.maxDay" :min="1" :max="3650" controls-position="right" />
                </el-form-item>
                <el-form-item :label="$t('pro.waf.logMaxSize')">
                    <el-input-number v-model="form.log.maxSize" :min="1" :max="1024" controls-position="right" />
                </el-form-item>
                <el-form-item :label="$t('pro.waf.secret')">
                    <el-input v-model="form.secret" show-password />
                </el-form-item>
                <el-form-item>
                    <el-button type="primary" :loading="saving" @click="save">{{ $t('pro.waf.configSave') }}</el-button>
                    <el-button :loading="maintaining" @click="runMaintenance">
                        {{ $t('pro.waf.maintainLogs') }}
                    </el-button>
                </el-form-item>
            </el-form>
        </el-card>

        <el-card shadow="never">
            <template #header>
                <div class="cdn-head">
                    <span>{{ $t('pro.waf.cdnTitle') }}</span>
                    <el-tag size="small" type="primary" effect="light">{{ $t('pro.waf.siteTag') }}</el-tag>
                </div>
            </template>
            <el-form label-width="180px" style="max-width: 740px">
                <el-form-item :label="$t('pro.waf.cdnSwitch')">
                    <el-switch v-model="form.cdnState" active-value="on" inactive-value="off" @change="saveCdnState(false)" />
                    <el-button class="ml-4" size="small" type="primary" plain @click="saveCdnState(true)">
                        {{ $t('pro.waf.applyToSites') }}
                    </el-button>
                    <div class="hint">{{ $t('pro.waf.cdnHelp') }}</div>
                </el-form-item>

                <el-form-item :label="$t('pro.waf.cdnHeaders')">
                    <div class="cdn-input-row">
                        <el-input
                            v-model="newHeader"
                            style="width: 280px"
                            clearable
                            :placeholder="$t('pro.waf.cdnHeaderPlaceholder')"
                            @keyup.enter="addHeader(newHeader)"
                        />
                        <el-button type="primary" plain @click="addHeader(newHeader)">
                            {{ $t('pro.waf.cdnAddHeader') }}
                        </el-button>
                    </div>
                    <div class="preset-row">
                        <span class="hint-inline">{{ $t('pro.waf.cdnPresets') }}</span>
                        <el-tag
                            v-for="h in presetHeaders"
                            :key="h"
                            size="small"
                            effect="plain"
                            class="preset-tag"
                            @click="addHeader(h)"
                        >
                            + {{ h }}
                        </el-tag>
                    </div>
                    <div class="header-tags mt-2" v-loading="loadingHeaders">
                        <el-tag
                            v-for="row in cdnHeaders"
                            :key="row.id || row.rule"
                            closable
                            class="mr-2 mb-1"
                            @close="removeHeader(row)"
                        >
                            {{ row.rule }}
                        </el-tag>
                        <span v-if="!cdnHeaders.length && !loadingHeaders" class="hint-inline">
                            {{ $t('pro.waf.noData') }}
                        </span>
                    </div>
                </el-form-item>
            </el-form>
        </el-card>
    </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue';
import { ElMessage } from 'element-plus';
import { useI18n } from 'vue-i18n';
import {
    applyWafGlobal,
    deleteWafRules,
    maintainWafLogs,
    saveWafRule,
    searchWafRules,
    updateWafGlobal,
    WafRule,
} from '@/xpack/api/waf';

const props = defineProps<{ config: Record<string, any>; node?: string }>();
const emit = defineEmits<{ reload: [] }>();
const { t } = useI18n();

const saving = ref(false);
const maintaining = ref(false);
const loadingHeaders = ref(false);
const cdnHeaders = ref<WafRule[]>([]);
const newHeader = ref('');

const presetHeaders = [
    'cf-connecting-ip',
    'x-forwarded-for',
    'x-real-ip',
    'ali-cdn-real-ip',
    'eo-connecting-ip',
    'true-client-ip',
];

const form = reactive({
    log: {
        state: 'on' as 'on' | 'off',
        maxDay: 180,
        maxSize: 1,
    },
    secret: '',
    cdnState: 'off' as 'on' | 'off',
});

watch(
    () => props.config,
    (cfg) => {
        const lg = cfg?.log || {};
        form.log.state = lg.state === 'off' ? 'off' : 'on';
        form.log.maxDay = Number(lg.maxDay) > 0 ? Number(lg.maxDay) : 180;
        form.log.maxSize = Number(lg.maxSize) > 0 ? Number(lg.maxSize) : 1;
        form.secret = cfg?.waf?.secret ?? '';
        form.cdnState = cfg?.cdn?.state === 'on' ? 'on' : 'off';
    },
    { immediate: true, deep: true },
);

const save = async () => {
    saving.value = true;
    try {
        await updateWafGlobal(
            {
                log: { ...(props.config?.log || {}), ...form.log },
                waf: { ...(props.config?.waf || {}), secret: form.secret },
            },
            props.node,
        );
        ElMessage.success(t('pro.waf.saveSuccess'));
        emit('reload');
    } finally {
        saving.value = false;
    }
};

const runMaintenance = async () => {
    maintaining.value = true;
    try {
        await maintainWafLogs(props.node);
        ElMessage.success(t('pro.waf.maintainSuccess'));
    } finally {
        maintaining.value = false;
    }
};

const saveCdnState = async (applyToAll: boolean) => {
    await updateWafGlobal(
        {
            cdn: { ...(props.config?.cdn || {}), state: form.cdnState },
        },
        props.node,
    );
    if (applyToAll) {
        await applyWafGlobal(['cdn'], props.node);
    }
    ElMessage.success(applyToAll ? t('pro.waf.applySuccess') : t('pro.waf.saveSuccess'));
    emit('reload');
};

const loadCdnHeaders = async () => {
    loadingHeaders.value = true;
    try {
        const res = await searchWafRules('cdn', '', '', 1, 0, props.node);
        cdnHeaders.value = res.data.items || [];
    } catch {
        cdnHeaders.value = [];
    } finally {
        loadingHeaders.value = false;
    }
};

const addHeader = async (raw: string) => {
    const h = (raw || '').trim().toLowerCase();
    if (!h) return;
    if (cdnHeaders.value.some((x) => String(x.rule || '').toLowerCase() === h)) {
        newHeader.value = '';
        return;
    }
    await saveWafRule('cdn', '', { state: 'on', rule: h, name: h }, '', props.node);
    newHeader.value = '';
    ElMessage.success(t('pro.waf.saveSuccess'));
    loadCdnHeaders();
};

const removeHeader = async (row: WafRule) => {
    const key = row.id || row.rule || '';
    if (!key) return;
    await deleteWafRules('cdn', '', [key], props.node);
    ElMessage.success(t('commons.msg.deleteSuccess'));
    loadCdnHeaders();
};

watch(
    () => props.node,
    () => loadCdnHeaders(),
);

onMounted(loadCdnHeaders);
</script>

<style scoped>
.cdn-head {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 600;
}
.cdn-input-row {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
}
.preset-row {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 8px;
    width: 100%;
}
.preset-tag {
    cursor: pointer;
}
.header-tags {
    width: 100%;
}
.hint {
    width: 100%;
    margin-top: 4px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
    line-height: 1.5;
}
.hint-inline {
    font-size: 12px;
    color: var(--el-text-color-secondary);
}
</style>
