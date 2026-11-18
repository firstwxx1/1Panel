<template>
    <div>
        <NodeTargetBar v-model="targetNode" @change="reset" />
        <el-card shadow="never">
            <el-tabs v-model="name" @tab-change="reset">
                <el-tab-pane v-for="n in names" :key="n" :label="$t('pro.waf.list_' + n)" :name="n" />
            </el-tabs>

            <div class="toolbar">
                <div class="toolbar-left">
                    <el-button type="primary" @click="openAdd">{{ $t('commons.button.add') }}</el-button>
                    <el-button type="danger" plain :disabled="selected.length === 0" @click="removeSelected">
                        {{ $t('commons.button.delete') }}
                    </el-button>
                </div>
                <div class="toolbar-right">
                    <el-input
                        v-model="keyword"
                        style="width: 240px"
                        clearable
                        :placeholder="$t('commons.button.search')"
                        @change="reset"
                        @keyup.enter="reset"
                    >
                        <template #prefix>
                            <el-icon><Search /></el-icon>
                        </template>
                    </el-input>
                    <el-button :icon="Refresh" circle @click="load" />
                </div>
            </div>

            <el-table
                :data="items"
                v-loading="loading"
                size="small"
                @selection-change="(v: WafRule[]) => (selected = v)"
            >
                <el-table-column type="selection" width="45" />
                <el-table-column :label="$t('pro.waf.rule')" min-width="240" show-overflow-tooltip>
                    <template #default="{ row }">
                        <span class="mono">{{ row.rule }}</span>
                    </template>
                </el-table-column>
                <el-table-column prop="name" :label="$t('pro.waf.remark')" min-width="160" show-overflow-tooltip />
                <el-table-column :label="$t('pro.waf.status')" width="100">
                    <template #default="{ row }">
                        <el-switch :model-value="row.state === 'on'" @change="(v: any) => toggle(row, Boolean(v))" />
                    </template>
                </el-table-column>
                <el-table-column :label="$t('commons.table.operate')" width="140" fixed="right">
                    <template #default="{ row }">
                        <el-button link type="primary" @click="openEdit(row)">{{ $t('commons.button.edit') }}</el-button>
                        <el-button link type="danger" @click="remove([row])">
                            {{ $t('commons.button.delete') }}
                        </el-button>
                    </template>
                </el-table-column>
            </el-table>

            <div class="pager">
                <el-pagination
                    v-model:current-page="page"
                    v-model:page-size="size"
                    :total="total"
                    :page-sizes="[20, 50, 100]"
                    layout="total, sizes, prev, pager, next, jumper"
                    small
                    @change="load"
                />
            </div>

            <el-dialog
                v-model="dialog"
                :title="editing ? $t('commons.button.edit') : $t('commons.button.add')"
                width="480px"
            >
                <el-form label-position="top">
                    <el-form-item :label="$t('pro.waf.rule')">
                        <el-input v-model="form.rule" type="textarea" :rows="3" />
                    </el-form-item>
                    <el-form-item :label="$t('pro.waf.remark')">
                        <el-input v-model="form.name" />
                    </el-form-item>
                </el-form>
                <template #footer>
                    <el-button @click="dialog = false">{{ $t('commons.button.cancel') }}</el-button>
                    <el-button type="primary" @click="submit">{{ $t('commons.button.confirm') }}</el-button>
                </template>
            </el-dialog>
        </el-card>
    </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { ElMessage } from 'element-plus';
import { Refresh, Search } from '@element-plus/icons-vue';
import { useI18n } from 'vue-i18n';
import { GlobalStore } from '@/store';
import NodeTargetBar from '@/xpack/components/NodeTargetBar.vue';
import { deleteWafRules, saveWafRule, searchWafRules, WafRule } from '@/xpack/api/waf';

const { t } = useI18n();
const globalStore = GlobalStore();
const targetNode = ref(globalStore.currentNode || 'local');
const names = ['ipBlack', 'ipWhite', 'urlBlack', 'urlWhite', 'uaBlack', 'uaWhite'];
const name = ref('ipBlack');
const keyword = ref('');
const page = ref(1);
const size = ref(20);
const total = ref(0);
const loading = ref(false);
const items = ref<WafRule[]>([]);
const selected = ref<WafRule[]>([]);
const dialog = ref(false);
const editing = ref(false);
const editingRow = ref<WafRule | null>(null);
const oldId = ref('');
const form = reactive<{ rule: string; name: string }>({ rule: '', name: '' });

// Always use unique id or rule pattern (never remark `name`, which can collide across rows)
const ruleKeyOf = (r: WafRule) => r.id || r.rule || '';

const inferRuleType = (listName: string, ruleVal: string, prevType?: string) => {
    if (listName.startsWith('ip')) {
        if (ruleVal.includes('/') || ruleVal.includes('-')) return 'ipArr';
        if (ruleVal.includes(':')) return 'ipv6';
        return prevType && prevType.startsWith('ip') ? prevType : 'ipv4';
    }
    if (listName.startsWith('url')) return 'url';
    return 'ua';
};

const load = async () => {
    loading.value = true;
    try {
        const res = await searchWafRules(name.value, '', keyword.value, page.value, size.value, targetNode.value);
        items.value = res.data.items || [];
        total.value = res.data.total || 0;
    } catch {
        items.value = [];
        total.value = 0;
    } finally {
        loading.value = false;
    }
};

const reset = () => {
    page.value = 1;
    selected.value = [];
    load();
};

const openAdd = () => {
    editing.value = false;
    editingRow.value = null;
    oldId.value = '';
    Object.assign(form, { rule: '', name: '' });
    dialog.value = true;
};

const openEdit = (row: WafRule) => {
    editing.value = true;
    editingRow.value = { ...row };
    oldId.value = ruleKeyOf(row);
    Object.assign(form, { rule: row.rule || '', name: row.name || '' });
    dialog.value = true;
};

const submit = async () => {
    const ruleVal = (form.rule || '').trim();
    if (!ruleVal) return;
    const prev = editing.value ? editingRow.value : null;
    const type = inferRuleType(name.value, ruleVal, prev?.type);
    const entry: WafRule = {
        ...(prev || {}),
        state: prev?.state || 'on',
        type,
        rule: ruleVal,
        name: (form.name || '').trim() || ruleVal,
    };
    await saveWafRule(name.value, '', entry, oldId.value, targetNode.value);
    dialog.value = false;
    ElMessage.success(t('pro.waf.saveSuccess'));
    load();
};

const toggle = async (row: WafRule, on: boolean) => {
    await saveWafRule(name.value, '', { ...row, state: on ? 'on' : 'off' }, ruleKeyOf(row), targetNode.value);
    load();
};

const remove = async (rows: WafRule[]) => {
    const ids = rows.map(ruleKeyOf).filter(Boolean);
    if (!ids.length) return;
    await deleteWafRules(name.value, '', ids, targetNode.value);
    ElMessage.success(t('commons.msg.deleteSuccess'));
    load();
};
const removeSelected = () => remove(selected.value);

onMounted(load);
</script>

<style scoped>
.toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 8px;
    margin-bottom: 12px;
}

.toolbar-left,
.toolbar-right {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
}

.toolbar-left :deep(.el-button + .el-button) {
    margin-left: 0;
}

.pager {
    display: flex;
    justify-content: flex-end;
    margin-top: 12px;
}

.mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
</style>
