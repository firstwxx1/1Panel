<template>
    <div>
        <el-card shadow="never" class="unknown-card mb-3">
            <div class="unknown-row">
                <div>
                    <div class="unknown-title">
                        <span>{{ $t('pro.waf.unknownSiteTitle') }}</span>
                        <el-tag size="small" type="success" effect="light" class="ml-2">
                            {{ $t('pro.waf.globalTag') }}
                        </el-tag>
                    </div>
                    <div class="hint mt-1">{{ $t('pro.waf.unknownSiteDesc') }}</div>
                </div>
                <div class="unknown-controls">
                    <el-radio-group
                        v-model="unknownForm.code"
                        size="small"
                        :disabled="unknownForm.state !== 'on'"
                        @change="saveUnknown"
                    >
                        <el-radio-button :value="403">{{ $t('pro.waf.code403') }}</el-radio-button>
                        <el-radio-button :value="444">{{ $t('pro.waf.code444') }}</el-radio-button>
                    </el-radio-group>
                    <el-switch
                        v-model="unknownForm.state"
                        active-value="on"
                        inactive-value="off"
                        @change="saveUnknown"
                    />
                </div>
            </div>
        </el-card>

        <div class="bar">
            <span class="hint">{{ $t('pro.waf.defaultRulesHint') }}</span>
            <el-button type="primary" plain size="small" @click="apply">{{ $t('pro.waf.applyToSites') }}</el-button>
        </div>
        <el-table :data="rows" size="small">
            <el-table-column :label="$t('pro.waf.ruleName')" min-width="220">
                <template #default="{ row }">{{ label(row.key) }}</template>
            </el-table-column>
            <el-table-column :label="$t('pro.waf.status')" width="120">
                <template #default="{ row }">
                    <el-switch v-model="row.item.state" active-value="on" inactive-value="off" @change="save(row.key)" />
                </template>
            </el-table-column>
            <el-table-column :label="$t('pro.waf.action')" width="220">
                <template #default="{ row }">
                    <el-select v-if="row.item.action" v-model="row.item.action" size="small" @change="save(row.key)">
                        <template v-if="isAllowRule(row.key)">
                            <el-option value="allow" :label="$t('pro.waf.act_allow')" />
                            <el-option value="deny" :label="$t('pro.waf.act_deny')" />
                        </template>
                        <template v-else>
                            <el-option value="deny" :label="$t('pro.waf.act_deny')" />
                            <el-option value="five_seconds" :label="$t('pro.waf.act_five_seconds')" />
                            <el-option value="captcha" :label="$t('pro.waf.act_captcha')" />
                            <el-option value="observe" :label="$t('pro.waf.act_observe')" />
                        </template>
                    </el-select>
                </template>
            </el-table-column>
        </el-table>
    </div>
</template>

<script setup lang="ts">
import { computed, reactive, watch } from 'vue';
import { ElMessage } from 'element-plus';
import { useI18n } from 'vue-i18n';
import { applyWafGlobal, updateWafGlobal } from '@/xpack/api/waf';

const props = defineProps<{ config: Record<string, any>; node?: string }>();
const emit = defineEmits<{ reload: [] }>();
const { t, te } = useI18n();

const unknownForm = reactive({
    state: 'off' as 'on' | 'off',
    code: 444 as 403 | 444,
});

watch(
    () => props.config?.unknownWebsite,
    (uw) => {
        unknownForm.state = uw?.state === 'on' ? 'on' : 'off';
        unknownForm.code = Number(uw?.code) === 403 ? 403 : 444;
    },
    { immediate: true, deep: true },
);

const saveUnknown = async () => {
    await updateWafGlobal(
        {
            unknownWebsite: {
                ...(props.config?.unknownWebsite || {}),
                state: unknownForm.state,
                type: 'unknownWebsite',
                action: 'deny',
                code: unknownForm.code,
                res: 'unknown',
            },
        },
        props.node,
    );
    ElMessage.success(t('pro.waf.saveSuccess'));
    emit('reload');
};

const keys = [
    'sql',
    'xss',
    'args',
    'cookie',
    'header',
    'fileExt',
    'methodWhite',
    'defaultUaBlack',
    'defaultUrlBlack',
    'defaultIpBlack',
    'vuln',
    'allowSpider',
    'strict',
];

const isAllowRule = (key: string) => key === 'methodWhite' || key === 'allowSpider';

const rows = computed(() => keys.filter((k) => props.config[k]).map((k) => ({ key: k, item: props.config[k] })));
const label = (key: string) => (te('pro.waf.mod_' + key) ? t('pro.waf.mod_' + key) : key);

const save = async (key: string) => {
    await updateWafGlobal({ [key]: props.config[key] }, props.node);
    ElMessage.success(t('pro.waf.saveSuccess'));
    emit('reload');
};

const apply = async () => {
    await applyWafGlobal(
        rows.value.map((r) => r.key),
        props.node,
    );
    ElMessage.success(t('pro.waf.applySuccess'));
};
</script>

<style scoped>
.unknown-card {
    border: 1px solid var(--el-border-color-lighter);
    background: var(--el-fill-color-lighter);
}

.unknown-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 12px;
}

.unknown-title {
    font-weight: 600;
    font-size: 14px;
    display: flex;
    align-items: center;
    color: var(--el-text-color-primary);
}

.unknown-controls {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 14px;
}

.bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 8px;
    padding: 8px 12px;
    margin-bottom: 12px;
    background: var(--el-fill-color-light);
    border-radius: 6px;
}

.hint {
    font-size: 12px;
    color: var(--el-text-color-secondary);
}
</style>
