<template>
    <div>
        <NodeTargetBar v-model="targetNode" @change="onNodeChange" />
        <el-card shadow="never" v-loading="loading">
            <div class="mb-4">
                <el-radio-group v-model="scope">
                    <el-radio-button value="global">{{ $t('pro.monitor.globalSettings') }}</el-radio-button>
                    <el-radio-button value="site">{{ $t('pro.monitor.siteSettings') }}</el-radio-button>
                </el-radio-group>
            </div>
            <div v-if="cfg" class="form">
                <template v-if="scope === 'global'">
                    <div class="row">
                        <span class="label">{{ $t('pro.monitor.state') }}</span>
                        <div class="control">
                            <el-switch :model-value="cfg.enabled" @change="toggleGlobal" />
                        </div>
                    </div>
                    <div v-for="f in numberFields" :key="f.key" class="row">
                        <span class="label">{{ f.label }}</span>
                        <div class="control">
                            <el-input :model-value="String((cfg as any)[f.key])" readonly class="field">
                                <template #append>
                                    <el-button :icon="Setting" @click="edit(f, 'global')">{{ $t('pro.monitor.set') }}</el-button>
                                </template>
                            </el-input>
                            <div class="hint">{{ f.hint }}</div>
                        </div>
                    </div>
                    <div v-for="f in listFields" :key="f.key" class="row">
                        <span class="label">{{ f.label }}</span>
                        <div class="control">
                            <el-input :model-value="join((cfg as any)[f.key])" readonly class="field">
                                <template #append>
                                    <el-button :icon="Setting" @click="edit(f, 'global')">{{ $t('pro.monitor.set') }}</el-button>
                                </template>
                            </el-input>
                            <div v-if="f.hint" class="hint">{{ f.hint }}</div>
                        </div>
                    </div>
                    <div class="row">
                        <span class="label">{{ $t('pro.monitor.cdn') }}</span>
                        <div class="control">
                            <el-switch v-model="cfg.cdn" @change="save" />
                            <div class="hint">{{ $t('pro.monitor.cdnHelp') }}</div>
                        </div>
                    </div>
                    <div class="row">
                        <span class="label">{{ $t('pro.monitor.rtTitle') }}</span>
                        <div class="control">
                            <el-tag v-if="hasRt" type="success" effect="light">{{ $t('pro.monitor.rtOn') }}</el-tag>
                            <el-button v-else type="primary" plain @click="enableRt">{{ $t('pro.monitor.rtEnable') }}</el-button>
                            <div v-if="!hasRt" class="hint">{{ $t('pro.monitor.rtOff') }}</div>
                        </div>
                    </div>
                </template>
                <template v-else>
                    <div class="row">
                        <span class="label">{{ $t('pro.monitor.website') }}</span>
                        <div class="control">
                            <SiteSelect v-model="site" :sites="siteList" />
                        </div>
                    </div>
                    <template v-if="site">
                        <div class="row">
                            <span class="label">{{ $t('pro.monitor.state') }}</span>
                            <div class="control">
                                <el-switch v-model="cfg.sites[site]" :disabled="!cfg.enabled" @change="save" />
                                <div v-if="!cfg.enabled" class="hint">{{ $t('pro.monitor.needGlobal') }}</div>
                            </div>
                        </div>
                        <div v-for="f in listFields" :key="f.key" class="row">
                            <span class="label">{{ f.label }}</span>
                            <div class="control">
                                <el-input :model-value="join(rules[f.key])" readonly class="field">
                                    <template #append>
                                        <el-button :icon="Setting" @click="edit(f, 'site')">{{ $t('pro.monitor.set') }}</el-button>
                                    </template>
                                </el-input>
                                <div class="hint">{{ $t('pro.monitor.siteExtra') }}</div>
                            </div>
                        </div>
                        <div class="row">
                            <span class="label">{{ $t('pro.monitor.cdn') }}</span>
                            <div class="control">
                                <el-switch v-model="rules.cdn" @change="save" />
                            </div>
                        </div>
                        <div class="row">
                            <span class="label">{{ $t('pro.monitor.clearSite') }}</span>
                            <div class="control">
                                <el-button type="danger" plain @click="clear">{{ $t('pro.monitor.clearLog') }}</el-button>
                            </div>
                        </div>
                    </template>
                    <el-empty v-else :image-size="48" :description="$t('pro.monitor.noSite')" />
                </template>
            </div>

            <el-dialog v-model="dialog" :title="editing?.label" width="480px">
                <el-input-number v-if="editing?.kind === 'number'" v-model="numText" :min="1" :step="editing.step" style="width: 100%" />
                <el-input v-else v-model="text" type="textarea" :rows="6" />
                <div class="hint-dialog">{{ editing?.help }}</div>
                <template #footer>
                    <el-button @click="dialog = false">{{ $t('commons.button.cancel') }}</el-button>
                    <el-button type="primary" @click="commit">{{ $t('commons.button.confirm') }}</el-button>
                </template>
            </el-dialog>
        </el-card>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { Setting } from '@element-plus/icons-vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { useI18n } from 'vue-i18n';
import { GlobalStore } from '@/store';
import NodeTargetBar from '@/xpack/components/NodeTargetBar.vue';
import SiteSelect from './SiteSelect.vue';
import {
    monClear,
    monEnableRt,
    monGetSettings,
    monSaveSettings,
    MonRules,
    MonSettings,
    MonSiteItem,
} from '@/xpack/api/monitor';

const emit = defineEmits(['changed']);
const { t } = useI18n();
const globalStore = GlobalStore();
const targetNode = ref(globalStore.currentNode || 'local');
const loading = ref(false);
const scope = ref<'global' | 'site'>('global');
const cfg = ref<MonSettings | null>(null);
const siteList = ref<MonSiteItem[]>([]);
const hasRt = ref(false);
const site = ref('');

interface Field {
    key: string;
    label: string;
    kind: 'number' | 'list';
    help?: string;
    hint?: string;
    step?: number;
}
const numberFields = computed<Field[]>(() => [
    { key: 'retentionDays', label: t('pro.monitor.retention'), kind: 'number', step: 1, hint: t('pro.monitor.retentionHelp') },
    { key: 'detailMax', label: t('pro.monitor.detailMax'), kind: 'number', step: 10000, hint: t('pro.monitor.detailMaxHelp') },
]);
const listFields = computed<Field[]>(() => [
    { key: 'excludeStatus', label: t('pro.monitor.excludeStatus'), kind: 'list', help: t('pro.monitor.listHelp') },
    { key: 'excludeSuffix', label: t('pro.monitor.excludeSuffix'), kind: 'list', help: t('pro.monitor.excludeSuffixHelp') },
    { key: 'excludeUri', label: t('pro.monitor.excludeUri'), kind: 'list', help: t('pro.monitor.excludeUriHelp') },
    { key: 'excludeIps', label: t('pro.monitor.excludeIps'), kind: 'list', help: t('pro.monitor.excludeIpsHelp') },
    { key: 'excludeUa', label: t('pro.monitor.excludeUa'), kind: 'list', help: t('pro.monitor.excludeUaHelp') },
]);

const empty = (): MonRules => ({ excludeStatus: [], excludeSuffix: [], excludeUri: [], excludeIps: [], excludeUa: [], cdn: false });
const rules = computed<any>(() => {
    if (!cfg.value || !site.value) return empty();
    if (!cfg.value.siteRules) cfg.value.siteRules = {};
    if (!cfg.value.siteRules[site.value]) cfg.value.siteRules[site.value] = empty();
    return cfg.value.siteRules[site.value];
});
const join = (l?: string[]) => (l || []).join(',');

const load = async () => {
    loading.value = true;
    try {
        const d = (await monGetSettings(targetNode.value)).data;
        siteList.value = d.siteList || [];
        hasRt.value = d.hasRt;
        d.sites = d.sites || {};
        d.siteRules = d.siteRules || {};
        for (const k of ['excludeIps', 'excludeSuffix', 'staticSuffix', 'excludeStatus', 'excludeUri', 'excludeUa'] as const) {
            (d as any)[k] = d[k] || [];
        }
        cfg.value = d;
        if (!site.value || !siteList.value.some((s) => s.alias === site.value)) {
            site.value = siteList.value[0]?.alias || '';
        }
    } catch {
        cfg.value = null;
        siteList.value = [];
    } finally {
        loading.value = false;
    }
};

const onNodeChange = () => {
    site.value = '';
    load();
};

const save = async (tip: any = true) => {
    if (!cfg.value) return;
    const { siteList: _s, hasRt: _h, ...rest } = cfg.value;
    const sr: Record<string, MonRules> = {};
    for (const [k, v] of Object.entries(rest.siteRules || {})) {
        if (v.excludeStatus.length || v.excludeSuffix.length || v.excludeUri.length || v.excludeIps.length || v.excludeUa.length || v.cdn) {
            sr[k] = v;
        }
    }
    await monSaveSettings({ ...rest, siteRules: sr }, targetNode.value);
    if (tip) ElMessage.success(t('pro.monitor.saved'));
    emit('changed');
};

const toggleGlobal = async (on: any) => {
    if (!cfg.value) return;
    cfg.value.enabled = !!on;
    if (on) siteList.value.forEach((s) => (cfg.value!.sites[s.alias] = true));
    await save();
};

const dialog = ref(false);
const editing = ref<Field | null>(null);
const editScope = ref<'global' | 'site'>('global');
const text = ref('');
const numText = ref(0);
const edit = (f: Field, sc: 'global' | 'site') => {
    editing.value = f;
    editScope.value = sc;
    const target: any = sc === 'global' ? cfg.value : rules.value;
    if (f.kind === 'number') numText.value = target[f.key];
    else text.value = (target[f.key] || []).join('\n');
    dialog.value = true;
};
const commit = async () => {
    const f = editing.value;
    if (!f || !cfg.value) return;
    const target: any = editScope.value === 'global' ? cfg.value : rules.value;
    if (f.kind === 'number') target[f.key] = numText.value;
    else
        target[f.key] = text.value
            .split(/[\n,]/)
            .map((x) => x.trim())
            .filter(Boolean);
    dialog.value = false;
    await save();
};

const enableRt = async () => {
    await monEnableRt(targetNode.value);
    ElMessage.success(t('pro.monitor.rtOk'));
    load();
};
const clear = async () => {
    await ElMessageBox.confirm(t('pro.monitor.clearConfirm'), t('pro.monitor.clearLog'), { type: 'warning' });
    await monClear(site.value, targetNode.value);
    ElMessage.success(t('pro.monitor.saved'));
};
onMounted(load);
</script>

<style scoped>
.row {
    display: flex;
    align-items: flex-start;
    gap: 16px;
    margin-bottom: 18px;
}
.label {
    width: 170px;
    flex-shrink: 0;
    padding-top: 6px;
    font-size: 13.5px;
    color: var(--el-text-color-regular);
}
.control {
    flex: 1;
    min-width: 0;
}
.field {
    width: 540px;
    max-width: 100%;
}
.hint {
    margin-top: 6px;
    font-size: 12px;
    line-height: 1.5;
    color: var(--el-text-color-secondary);
}
.hint-dialog {
    margin-top: 8px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
}

@media (max-width: 640px) {
    .row {
        flex-direction: column;
        gap: 6px;
    }
    .label {
        width: auto;
        padding-top: 0;
    }
}
</style>
