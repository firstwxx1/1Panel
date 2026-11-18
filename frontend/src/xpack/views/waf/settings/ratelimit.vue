<template>
    <el-row :gutter="20" class="ratelimit-wrap">
        <el-col :xs="24" :sm="6" class="menu-col">
            <el-menu :default-active="active" class="menu">
                <el-menu-item v-for="m in menus" :key="m.key" :index="m.key" @click="active = m.key">
                    {{ $t('pro.waf.' + m.label) }}
                </el-menu-item>
            </el-menu>
        </el-col>
        <el-col :xs="24" :sm="18">
            <div v-if="item" class="panel">
                <div class="panel-header">
                    <div class="panel-title-wrap">
                        <span class="panel-title">{{ activeMenuLabel }}</span>
                        <el-tag size="small" :type="isSiteRule ? 'primary' : 'success'" effect="light">
                            {{ isSiteRule ? $t('pro.waf.siteTag') : $t('pro.waf.globalTag') }}
                        </el-tag>
                    </div>
                    <el-switch v-model="item.state" active-value="on" inactive-value="off" size="large" />
                </div>

                <div class="summary-banner">
                    {{ summary }}
                </div>

                <el-form label-width="130px" class="mt-4">
                    <el-form-item v-if="active === 'cc'" :label="$t('pro.waf.mode')" required>
                        <el-select v-model="item.mode" style="width: 240px">
                            <el-option value="uri" :label="$t('pro.waf.urlMode')" />
                            <el-option value="global" :label="$t('pro.waf.globalMode')" />
                        </el-select>
                    </el-form-item>
                    <el-form-item :label="$t('pro.waf.period')" required>
                        <el-input-number
                            v-model="item.duration"
                            :min="1"
                            controls-position="right"
                            style="width: 140px"
                        />
                        <span class="unit">{{ $t('pro.waf.second') }}</span>
                    </el-form-item>
                    <el-form-item :label="$t('pro.waf.frequency')" required>
                        <el-input-number
                            v-model="item.threshold"
                            :min="1"
                            controls-position="right"
                            style="width: 140px"
                        />
                        <span class="unit">{{ $t('pro.waf.times') }}</span>
                    </el-form-item>
                    <el-form-item :label="$t('pro.waf.action')" required>
                        <el-select v-model="item.action" style="width: 240px">
                            <el-option v-for="a in actionOptions" :key="a" :value="a" :label="$t('pro.waf.act_' + a)" />
                        </el-select>
                    </el-form-item>
                    <el-form-item
                        v-if="item.action === 'five_seconds' || item.action === 'captcha'"
                        :label="$t('pro.waf.tokenTimeOut')"
                    >
                        <el-input-number
                            v-model="item.tokenTimeOut"
                            :min="10"
                            :max="86400"
                            controls-position="right"
                            style="width: 140px"
                        />
                        <span class="unit">{{ $t('pro.waf.second') }}</span>
                        <div class="hint">{{ $t('pro.waf.tokenTimeOutHint') }}</div>
                    </el-form-item>
                    <el-form-item :label="$t('pro.waf.autoBlockIp')">
                        <el-switch v-model="item.ipBlock" active-value="on" inactive-value="off" />
                    </el-form-item>
                    <el-form-item v-if="item.ipBlock !== 'off'" :label="$t('pro.waf.blockTime')" required>
                        <el-input-number
                            v-model="item.ipBlockTime"
                            :min="1"
                            controls-position="right"
                            style="width: 140px"
                        />
                        <span class="unit">{{ $t('pro.waf.second') }}</span>
                    </el-form-item>
                    <el-form-item>
                        <el-button type="primary" :loading="saving" @click="save(false)">
                            {{ isSiteRule ? $t('pro.waf.saveDefault') : $t('pro.waf.configSave') }}
                        </el-button>
                        <el-button v-if="isSiteRule" type="primary" plain :loading="saving" @click="save(true)">
                            {{ $t('pro.waf.applyToSites') }}
                        </el-button>
                    </el-form-item>
                </el-form>

                <div v-if="active === 'cc'" class="note">
                    {{ $t('pro.waf.globalModeNote') }}<br />{{ $t('pro.waf.urlModeNote') }}
                </div>
                <div class="warn">{{ $t('pro.waf.blockWarning') }}</div>
            </div>
        </el-col>
    </el-row>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { ElMessage } from 'element-plus';
import { useI18n } from 'vue-i18n';
import { applyWafGlobal, updateWafGlobal } from '@/xpack/api/waf';

const props = defineProps<{ config: Record<string, any>; node?: string }>();
const emit = defineEmits<{ reload: [] }>();
const { t } = useI18n();

const menus = [
    { key: 'cc', label: 'accessRate' },
    { key: 'attackCount', label: 'attackRate' },
    { key: 'notFoundCount', label: 'notFoundRate' },
    { key: 'urlcc', label: 'urlRate' },
];

const actionOptions = ['deny', 'five_seconds', 'captcha', 'observe'];
const active = ref('cc');
const saving = ref(false);

const activeMenuLabel = computed(() => {
    const found = menus.find((m) => m.key === active.value);
    return found ? t('pro.waf.' + found.label) : '';
});

const defaultEntry = (key: string): Record<string, any> => ({
    state: 'off',
    type: key,
    ...(key === 'cc' ? { mode: 'global' } : {}),
    duration: 60,
    threshold: key === 'attackCount' ? 10 : key === 'notFoundCount' ? 30 : key === 'urlcc' ? 60 : 120,
    action: 'deny',
    code: 403,
    ipBlock: 'on',
    ipBlockTime: 600,
    tokenTimeOut: 1200,
});

const localItems = reactive<Record<string, Record<string, any>>>({
    cc: defaultEntry('cc'),
    attackCount: defaultEntry('attackCount'),
    notFoundCount: defaultEntry('notFoundCount'),
    urlcc: defaultEntry('urlcc'),
});

watch(
    () => props.config,
    (cfg) => {
        for (const m of menus) {
            const raw = cfg?.[m.key] || {};
            localItems[m.key] = {
                ...defaultEntry(m.key),
                ...raw,
                type: m.key,
                action: raw.action || 'deny',
                ipBlock: raw.ipBlock === 'off' ? 'off' : 'on',
                tokenTimeOut: Number(raw.tokenTimeOut) > 0 ? Number(raw.tokenTimeOut) : 1200,
            };
        }
    },
    { immediate: true, deep: true },
);

const item = computed(() => localItems[active.value]);
const isSiteRule = computed(() => active.value === 'cc' || active.value === 'urlcc');

const target = computed(() =>
    active.value === 'cc' && item.value?.mode === 'uri' ? t('pro.waf.targetUrl') : t('pro.waf.targetAny'),
);

const actionLabel = computed(() => {
    const a = item.value?.action || 'deny';
    return t('pro.waf.act_' + a);
});

const summary = computed(() =>
    t('pro.waf.summary', {
        period: item.value?.duration ?? 0,
        target: target.value,
        threshold: item.value?.threshold ?? 0,
        action: actionLabel.value,
    }),
);

const save = async (apply: boolean) => {
    saving.value = true;
    try {
        const payload = { ...item.value, type: active.value };
        await updateWafGlobal({ [active.value]: payload }, props.node);
        if (apply) await applyWafGlobal([active.value], props.node);
        ElMessage.success(apply ? t('pro.waf.applySuccess') : t('pro.waf.saveSuccess'));
        emit('reload');
    } finally {
        saving.value = false;
    }
};
</script>

<style scoped>
.ratelimit-wrap {
    padding-top: 4px;
}

.menu-col {
    border-right: 1px solid var(--el-border-color-lighter);
}

.menu {
    border-right: none;
    background: transparent;
}

.menu :deep(.el-menu-item) {
    height: 42px;
    line-height: 42px;
    margin-bottom: 4px;
    border-radius: 6px;
    font-size: 13px;
}

.menu :deep(.el-menu-item.is-active) {
    background: var(--el-color-primary-light-9);
    color: var(--el-color-primary);
    font-weight: 600;
}

.panel {
    padding: 2px 6px 4px;
}

.panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 12px;
}

.panel-title-wrap {
    display: flex;
    align-items: center;
    gap: 8px;
}

.panel-title {
    font-size: 15px;
    font-weight: 600;
    color: var(--el-text-color-primary);
}

.summary-banner {
    padding: 10px 14px;
    font-size: 13px;
    color: var(--el-text-color-regular);
    background: var(--el-fill-color-light);
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 6px;
}

.unit {
    margin-left: 8px;
    color: var(--el-text-color-secondary);
}

.hint {
    width: 100%;
    margin-top: 4px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
    line-height: 1.4;
}

.note {
    margin-top: 8px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
    line-height: 20px;
}

.warn {
    margin-top: 12px;
    padding: 8px 12px;
    font-size: 12px;
    color: var(--el-color-warning);
    background: var(--el-color-warning-light-9);
    border-radius: 6px;
}

@media (max-width: 767px) {
    .menu-col {
        border-right: none;
        border-bottom: 1px solid var(--el-border-color-lighter);
        padding-bottom: 8px;
        margin-bottom: 14px;
    }
    .panel {
        padding: 0;
    }
}
</style>
