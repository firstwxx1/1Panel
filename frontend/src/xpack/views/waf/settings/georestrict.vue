<template>
    <div class="geo-restrict">
        <div v-if="!siteMode" class="hint-bar mb-3">
            <span>{{ $t('pro.waf.geoHint') }}</span>
            <el-tag size="small" type="primary" effect="light">{{ $t('pro.waf.siteTag') }}</el-tag>
        </div>

        <el-form :label-width="siteMode ? '130px' : '150px'" style="max-width: 760px">
            <el-form-item :label="$t('pro.waf.geoTitle')">
                <el-switch v-model="form.state" active-value="on" inactive-value="off" @change="emitChange" />
            </el-form-item>

            <template v-if="form.state === 'on' || !siteMode">
                <el-form-item :label="$t('pro.waf.geoMode')">
                    <el-radio-group v-model="form.action" @change="emitChange">
                        <el-radio-button value="deny">{{ $t('pro.waf.geoDeny') }}</el-radio-button>
                        <el-radio-button value="allow">{{ $t('pro.waf.geoAllow') }}</el-radio-button>
                    </el-radio-group>
                </el-form-item>

                <el-form-item :label="$t('pro.waf.geoCountries')">
                    <el-select
                        v-model="form.rules"
                        multiple
                        filterable
                        allow-create
                        default-first-option
                        clearable
                        collapse-tags
                        collapse-tags-tooltip
                        :max-collapse-tags="8"
                        style="width: 100%"
                        :placeholder="$t('pro.waf.geoCountriesPlaceholder')"
                        @change="onCountryChange"
                    >
                        <el-option
                            v-for="c in GEO_COUNTRIES"
                            :key="c.iso"
                            :value="c.iso"
                            :label="isZh ? `${c.iso} - ${c.zh} (${c.en})` : `${c.iso} - ${c.en}`"
                        />
                    </el-select>
                    <div class="preset-row">
                        <el-button size="small" text bg @click="applyPreset('allow', ['CN'])">
                            {{ $t('pro.waf.geoPresetCN') }}
                        </el-button>
                        <el-button size="small" text bg @click="applyPreset('allow', ['CN', 'HK', 'MO', 'TW'])">
                            {{ $t('pro.waf.geoPresetGreaterCN') }}
                        </el-button>
                        <el-button size="small" text @click="clearCountries">
                            {{ $t('pro.waf.geoClear') }}
                        </el-button>
                    </div>
                </el-form-item>

                <el-divider content-position="left">{{ $t('pro.waf.geoProvinceTitle') }}</el-divider>

                <el-form-item :label="$t('pro.waf.geoProvinceMode')">
                    <el-radio-group v-model="form.provinceAction" @change="emitChange">
                        <el-radio value="deny">{{ $t('pro.waf.geoProvinceDeny') }}</el-radio>
                        <el-radio value="allow">{{ $t('pro.waf.geoProvinceAllow') }}</el-radio>
                    </el-radio-group>
                </el-form-item>

                <el-form-item :label="$t('pro.waf.geoProvinces')">
                    <el-select
                        v-model="form.provinceRules"
                        multiple
                        filterable
                        clearable
                        collapse-tags
                        collapse-tags-tooltip
                        :max-collapse-tags="8"
                        style="width: 100%"
                        :placeholder="$t('pro.waf.geoProvincesPlaceholder')"
                        @change="emitChange"
                    >
                        <el-option
                            v-for="p in CN_PROVINCES"
                            :key="p.value"
                            :value="p.value"
                            :label="isZh ? `${p.zh} (${p.en})` : p.en"
                        />
                    </el-select>
                </el-form-item>
            </template>

            <el-form-item v-if="!siteMode">
                <el-button type="primary" :loading="saving" @click="save(false)">
                    {{ $t('pro.waf.saveDefault') }}
                </el-button>
                <el-button type="primary" plain :loading="saving" @click="save(true)">
                    {{ $t('pro.waf.applyToSites') }}
                </el-button>
            </el-form-item>
        </el-form>
    </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { ElMessage } from 'element-plus';
import { useI18n } from 'vue-i18n';
import { applyWafGlobal, GeoRestrictConfig, updateWafGlobal } from '@/xpack/api/waf';
import { CN_PROVINCES, GEO_COUNTRIES, normalizeGeoRestrict } from './geo-options';

const props = withDefaults(
    defineProps<{
        config?: Record<string, any>;
        modelValue?: Record<string, any>;
        node?: string;
        siteMode?: boolean;
    }>(),
    {
        config: () => ({}),
        modelValue: undefined,
        node: '',
        siteMode: false,
    },
);

const emit = defineEmits<{
    (e: 'reload'): void;
    (e: 'update:modelValue', val: GeoRestrictConfig): void;
    (e: 'change', val: GeoRestrictConfig): void;
}>();

const { t, locale } = useI18n();
const isZh = computed(() => String(locale.value || '').toLowerCase().startsWith('zh'));
const saving = ref(false);

const form = reactive<GeoRestrictConfig>(normalizeGeoRestrict());

const syncFromProps = () => {
    const src = props.siteMode ? props.modelValue : props.config?.geoRestrict;
    Object.assign(form, normalizeGeoRestrict(src));
};

watch(
    () => (props.siteMode ? props.modelValue : props.config?.geoRestrict),
    () => syncFromProps(),
    { immediate: true, deep: true },
);

const buildPayload = (): GeoRestrictConfig => ({
    state: form.state,
    schemaVersion: 2,
    type: 'geoRestrict',
    res: 'geo',
    code: 403,
    action: form.action === 'allow' ? 'allow' : 'deny',
    rules: [...new Set((form.rules || []).map((c) => String(c || '').trim().toUpperCase()).filter(Boolean))],
    provinceAction: form.provinceAction === 'allow' ? 'allow' : 'deny',
    provinceRules: [...new Set((form.provinceRules || []).map((p) => String(p || '').trim()).filter(Boolean))],
});

const emitChange = () => {
    const payload = buildPayload();
    if (props.siteMode) {
        emit('update:modelValue', payload);
        emit('change', payload);
    }
};

const onCountryChange = (vals: string[]) => {
    form.rules = [...new Set((vals || []).map((v) => String(v || '').trim().toUpperCase()).filter(Boolean))];
    emitChange();
};

const applyPreset = (action: 'allow' | 'deny', codes: string[]) => {
    form.state = 'on';
    form.action = action;
    form.rules = [...codes];
    emitChange();
};

const clearCountries = () => {
    form.rules = [];
    emitChange();
};

const save = async (applyToAllSites: boolean) => {
    saving.value = true;
    try {
        const geoRestrict = buildPayload();
        await updateWafGlobal({ geoRestrict }, props.node);
        if (applyToAllSites) {
            await applyWafGlobal(['geoRestrict'], props.node);
        }
        ElMessage.success(applyToAllSites ? t('pro.waf.applySuccess') : t('pro.waf.saveSuccess'));
        emit('reload');
    } finally {
        saving.value = false;
    }
};
</script>

<style scoped>
.hint-bar {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
}
.preset-row {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 8px;
}
</style>
