<template>
    <div>
        <el-card>
            <el-alert v-if="kind === 'panels'" type="info" :closable="false" class="mb-3" :title="$t('pro.node.panelTip')" />
            <div class="bar">
                <div class="left">
                    <el-button type="primary" @click="openAdd">
                        {{ kind === 'nodes' ? $t('pro.node.addNode') : $t('pro.node.addPanel') }}
                    </el-button>
                    <el-button v-if="kind === 'nodes'" @click="notReady">{{ $t('pro.node.backupMaster') }}</el-button>
                    <el-button :disabled="!selected.length" @click="openGroup">{{ $t('pro.node.group') }}</el-button>
                    <el-button v-if="kind === 'nodes'" :disabled="!selected.length" @click="notReady">
                        {{ $t('pro.node.upgrade') }}
                    </el-button>
                    <el-button v-if="kind === 'nodes'" @click="notReady">{{ $t('pro.node.setting') }}</el-button>
                    <el-button :disabled="!selected.length" @click="removeSelected">{{ $t('pro.node.delete') }}</el-button>
                </div>
                <div class="right">
                    <el-select v-model="query.groupID" style="width: 140px" @change="filter">
                        <el-option :value="0" :label="$t('pro.node.allGroups')" />
                        <el-option v-for="g in groups" :key="g.id" :value="g.id" :label="groupLabel(g)" />
                    </el-select>
                    <el-input v-model="query.info" clearable :placeholder="$t('pro.node.search')" style="width: 180px" @keyup.enter="filter" @clear="filter">
                        <template #append>
                            <el-button :icon="Search" @click="filter" />
                        </template>
                    </el-input>
                    <el-button :icon="Refresh" circle @click="search" />
                    <el-select v-model="refresh" style="width: 120px">
                        <el-option :value="0" :label="$t('pro.node.noRefresh')" />
                        <el-option :value="10" label="10s" />
                        <el-option :value="30" label="30s" />
                        <el-option :value="60" label="60s" />
                    </el-select>
                </div>
            </div>

            <el-table :data="items" v-loading="loading" @selection-change="(v: NodeRow[]) => (selected = v)">
                <el-table-column type="selection" width="45" :selectable="(row: NodeRow) => !row.isLocal" />
                <el-table-column :label="$t('pro.node.name')" min-width="160">
                    <template #default="{ row }">
                        <div class="name">
                            <span>{{ row.isLocal ? $t('pro.node.master') : row.name }}</span>
                            <el-icon v-if="!row.isLocal" class="edit" @click="openEdit(row)"><Edit /></el-icon>
                        </div>
                        <div class="sub">{{ row.addr }}{{ kind === 'panels' && row.port ? ':' + row.port : '' }}</div>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('commons.table.status')" width="110">
                    <template #default="{ row }">
                        <el-tooltip :disabled="!row.message" :content="row.message" placement="top">
                            <el-tag :type="statusType(row.status)" effect="light">{{ statusText(row.status) }}</el-tag>
                        </el-tooltip>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('pro.node.group')" width="110" prop="groupBelong" />
                <el-table-column :label="$t('pro.node.version')" min-width="130">
                    <template #default="{ row }">
                        <template v-if="kind === 'nodes'">
                            <div>{{ $t('pro.node.pro') }}</div>
                            <div class="sub">{{ row.version }}</div>
                        </template>
                        <template v-else>{{ row.version || '-' }}</template>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('pro.node.resource')" min-width="170">
                    <template #default="{ row }">
                        <template v-if="row.cpuTotal">
                            <div>CPU: {{ row.cpuTotal }} {{ $t('pro.node.cores') }} ({{ row.cpuUsedPercent.toFixed(2) }}%)</div>
                            <div>
                                {{ $t('pro.node.memory') }}: {{ gb(row.memoryTotal) }} ({{ row.memoryUsedPercent.toFixed(2) }}%)
                            </div>
                        </template>
                        <template v-else>-</template>
                    </template>
                </el-table-column>
                <el-table-column v-if="kind === 'nodes'" :label="$t('pro.node.dataSync')" width="110">
                    <template #default>
                        <el-tag type="success" effect="light">{{ $t('pro.node.syncSuccess') }}</el-tag>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('pro.node.description')" min-width="140" show-overflow-tooltip prop="description" />
                <el-table-column :label="$t('pro.node.operate')" width="190" fixed="right">
                    <template #default="{ row }">
                        <template v-if="!row.isLocal">
                            <el-button link type="primary" @click="openEdit(row)">{{ $t('pro.node.edit') }}</el-button>
                            <el-button link type="primary" @click="removeOne(row)">{{ $t('pro.node.delete') }}</el-button>
                            <el-dropdown trigger="click" @command="(cmd: string) => more(cmd, row)">
                                <el-button link type="primary">{{ $t('pro.node.moreAction') }}</el-button>
                                <template #dropdown>
                                    <el-dropdown-menu>
                                        <el-dropdown-item command="test">{{ $t('pro.node.test') }}</el-dropdown-item>
                                        <el-dropdown-item v-if="kind === 'nodes'" command="restart">
                                            {{ $t('pro.node.restart') }}
                                        </el-dropdown-item>
                                        <el-dropdown-item v-if="kind === 'panels'" command="open">
                                            {{ $t('pro.node.open') }}
                                        </el-dropdown-item>
                                    </el-dropdown-menu>
                                </template>
                            </el-dropdown>
                        </template>
                    </template>
                </el-table-column>
                <template #empty>{{ $t('pro.node.noData') }}</template>
            </el-table>
            <div class="pager">
                <el-pagination
                    v-model:current-page="query.page"
                    v-model:page-size="query.pageSize"
                    :total="total"
                    :page-sizes="[20, 50, 100]"
                    layout="total, sizes, prev, pager, next, jumper"
                    @change="search"
                />
            </div>
        </el-card>

        <!-- add / edit -->
        <el-dialog v-model="formVisible" :title="formTitle" width="560px" :close-on-click-modal="false" destroy-on-close>
            <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
                <el-form-item v-if="kind === 'nodes' && !editing" :label="$t('pro.node.joinMode')">
                    <el-radio-group v-model="mode">
                        <el-radio-button value="command">{{ $t('pro.node.modeCommand') }}</el-radio-button>
                        <el-radio-button value="ssh">{{ $t('pro.node.modeSSH') }}</el-radio-button>
                    </el-radio-group>
                </el-form-item>
                <el-form-item :label="$t('pro.node.nodeName')" prop="name">
                    <el-input v-model="form.name" />
                    <span class="hint">{{ $t('pro.node.nodeNameRule') }}</span>
                </el-form-item>
                <el-form-item :label="$t('pro.node.group')" prop="groupID">
                    <el-select v-model="form.groupID" style="width: 100%">
                        <el-option v-for="g in groups" :key="g.id" :value="g.id" :label="groupLabel(g)" />
                    </el-select>
                </el-form-item>

                <template v-if="kind === 'nodes' && !editing">
                    <el-form-item :label="$t('pro.node.agentPort')">
                        <el-input-number v-model="form.agentPort" :min="1" :max="65535" controls-position="right" />
                    </el-form-item>
                    <template v-if="mode === 'ssh'">
                        <el-alert type="info" :closable="false" class="mb-3" :title="$t('pro.node.sshHelper')" />
                        <el-row :gutter="12">
                            <el-col :span="16">
                                <el-form-item :label="$t('pro.node.sshAddr')" prop="sshAddr">
                                    <el-input v-model="ssh.addr" />
                                </el-form-item>
                            </el-col>
                            <el-col :span="8">
                                <el-form-item :label="$t('pro.node.sshPort')">
                                    <el-input-number v-model="ssh.port" :min="1" :max="65535" controls-position="right" />
                                </el-form-item>
                            </el-col>
                        </el-row>
                        <el-form-item :label="$t('pro.node.sshUser')" prop="sshUser">
                            <el-input v-model="ssh.user" />
                        </el-form-item>
                        <el-form-item>
                            <el-radio-group v-model="ssh.authMode">
                                <el-radio value="password">{{ $t('pro.node.authPassword') }}</el-radio>
                                <el-radio value="key">{{ $t('pro.node.authKey') }}</el-radio>
                            </el-radio-group>
                        </el-form-item>
                        <el-form-item v-if="ssh.authMode === 'password'" :label="$t('pro.node.password')">
                            <el-input v-model="ssh.password" type="password" show-password />
                        </el-form-item>
                        <template v-else>
                            <el-form-item :label="$t('pro.node.privateKey')">
                                <el-input v-model="ssh.privateKey" type="textarea" :rows="4" />
                            </el-form-item>
                            <el-form-item :label="$t('pro.node.passPhrase')">
                                <el-input v-model="ssh.passPhrase" type="password" show-password />
                            </el-form-item>
                        </template>
                    </template>
                </template>

                <template v-if="kind === 'panels'">
                    <el-row :gutter="12">
                        <el-col :span="8">
                            <el-form-item :label="$t('pro.node.protocol')">
                                <el-select v-model="form.protocol">
                                    <el-option value="http" label="http" />
                                    <el-option value="https" label="https" />
                                </el-select>
                            </el-form-item>
                        </el-col>
                        <el-col :span="10">
                            <el-form-item :label="$t('pro.node.panelAddr')" prop="addr">
                                <el-input v-model="form.addr" />
                            </el-form-item>
                        </el-col>
                        <el-col :span="6">
                            <el-form-item :label="$t('pro.node.panelPort')" prop="port">
                                <el-input-number v-model="form.port" :min="1" :max="65535" :controls="false" style="width: 100%" />
                            </el-form-item>
                        </el-col>
                    </el-row>
                    <el-form-item :label="$t('pro.node.entrance')">
                        <el-input v-model="form.entrance" />
                    </el-form-item>
                    <el-form-item :label="$t('pro.node.apiKey')" :prop="editing ? '' : 'apiKey'">
                        <el-input v-model="form.apiKey" type="password" show-password :placeholder="editing ? $t('pro.node.apiKeyKeep') : ''" />
                        <span class="hint">{{ $t('pro.node.apiKeyHelper') }}</span>
                    </el-form-item>
                </template>

                <el-form-item :label="$t('pro.node.description')">
                    <el-input v-model="form.description" type="textarea" :rows="2" maxlength="200" />
                </el-form-item>
            </el-form>
            <template #footer>
                <el-button @click="formVisible = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" :loading="saving" @click="submit">{{ $t('commons.button.confirm') }}</el-button>
            </template>
        </el-dialog>

        <!-- join command -->
        <el-dialog v-model="cmdVisible" :title="$t('pro.node.commandTitle')" width="640px">
            <el-alert type="info" :closable="false" class="mb-3" :title="$t('pro.node.commandHelper')" />
            <el-input :model-value="command" type="textarea" :rows="3" readonly />
            <template #footer>
                <el-button @click="cmdVisible = false">{{ $t('commons.button.close') }}</el-button>
                <el-button type="primary" @click="copyCommand">{{ $t('pro.node.copy') }}</el-button>
            </template>
        </el-dialog>

        <!-- group -->
        <el-dialog v-model="groupVisible" :title="$t('pro.node.changeGroup')" width="420px">
            <el-select v-model="targetGroup" style="width: 100%">
                <el-option v-for="g in groups" :key="g.id" :value="g.id" :label="groupLabel(g)" />
            </el-select>
            <template #footer>
                <el-button @click="groupVisible = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" :disabled="!targetGroup" @click="applyGroup">{{ $t('commons.button.confirm') }}</el-button>
            </template>
        </el-dialog>
    </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { ElMessage, ElMessageBox, FormInstance } from 'element-plus';
import { Edit, Refresh, Search } from '@element-plus/icons-vue';
import { useI18n } from 'vue-i18n';
import { getGroupList } from '@/api/modules/group';
import {
    changeNodeGroup,
    createNode,
    deleteNodes,
    NodeCreate,
    NodeRow,
    nodeJoinCommand,
    restartNode,
    searchNodes,
    testNode,
    updateNode,
} from '@/xpack/api/node';

const props = defineProps<{ kind: 'nodes' | 'panels' }>();
const { t } = useI18n();

const query = reactive({ page: 1, pageSize: 20, info: '', groupID: 0 });
const items = ref<NodeRow[]>([]);
const total = ref(0);
const loading = ref(false);
const selected = ref<NodeRow[]>([]);
const groups = ref<{ id: number; name: string; isDefault: boolean }[]>([]);
const refresh = ref(0);
let timer: number | undefined;

const groupLabel = (g: { name: string; isDefault: boolean }) => (g.isDefault && g.name.toLowerCase() === 'default' ? '默认' : g.name);

const statusType = (s: string) => {
    if (s === 'Healthy') return 'success';
    if (s === 'Installing' || s === 'Pending') return 'warning';
    return 'danger';
};
const statusText = (s: string) => t('pro.node.status' + s);
const gb = (n: number) => (n / 1024 / 1024 / 1024).toFixed(2) + ' GB';
const notReady = () => ElMessage.info(t('pro.node.notReady'));

const search = async (silent = false) => {
    if (silent !== true) loading.value = true;
    try {
        const res = await searchNodes(props.kind, { ...query, groupID: query.groupID || undefined });
        items.value = res.data.items || [];
        total.value = res.data.total;
    } finally {
        loading.value = false;
    }
};
const filter = () => {
    query.page = 1;
    search();
};

// --- form ---
const formVisible = ref(false);
const editing = ref(false);
const saving = ref(false);
const mode = ref<'command' | 'ssh'>('command');
const formRef = ref<FormInstance>();
const blankForm = () => ({
    id: 0,
    name: '',
    groupID: groups.value.find((g) => g.isDefault)?.id || 0,
    description: '',
    agentPort: 9999,
    addr: '',
    port: 9999 as number | undefined,
    protocol: 'http',
    entrance: '',
    apiKey: '',
});
const form = reactive(blankForm());
const ssh = reactive({ addr: '', port: 22, user: 'root', authMode: 'password', password: '', privateKey: '', passPhrase: '' });

const nameRule = /^[一-龥A-Za-z0-9_-]{1,30}$/;
const rules = computed(() => ({
    name: [
        {
            required: true,
            trigger: 'blur',
            validator: (_: any, v: string, cb: any) => (nameRule.test(v) && v !== 'local' ? cb() : cb(new Error(t('pro.node.nodeNameRule')))),
        },
    ],
    sshAddr: [{ required: mode.value === 'ssh', validator: (_: any, __: string, cb: any) => (mode.value !== 'ssh' || ssh.addr ? cb() : cb(new Error(' '))), trigger: 'blur' }],
    sshUser: [{ required: mode.value === 'ssh', validator: (_: any, __: string, cb: any) => (mode.value !== 'ssh' || ssh.user ? cb() : cb(new Error(' '))), trigger: 'blur' }],
    addr: [{ required: props.kind === 'panels', message: ' ', trigger: 'blur' }],
    port: [{ required: props.kind === 'panels', message: ' ', trigger: 'blur' }],
    apiKey: [{ required: props.kind === 'panels', message: ' ', trigger: 'blur' }],
}));

const formTitle = computed(() => {
    if (editing.value) return t('pro.node.edit');
    return props.kind === 'nodes' ? t('pro.node.addNode') : t('pro.node.addPanel');
});

const openAdd = () => {
    editing.value = false;
    mode.value = 'command';
    Object.assign(form, blankForm());
    if (props.kind === 'panels') form.port = 9999;
    Object.assign(ssh, { addr: '', port: 22, user: 'root', authMode: 'password', password: '', privateKey: '', passPhrase: '' });
    formVisible.value = true;
};

const openEdit = (row: NodeRow) => {
    editing.value = true;
    Object.assign(form, blankForm(), {
        id: row.id,
        name: row.name,
        groupID: row.groupID,
        description: row.description,
        addr: row.addr,
        port: row.port,
        protocol: row.protocol || 'http',
        entrance: row.securityEntrance,
        apiKey: '',
    });
    formVisible.value = true;
};

const cmdVisible = ref(false);
const command = ref('');
const copyCommand = async () => {
    try {
        await navigator.clipboard.writeText(command.value);
    } catch {
        const ta = document.createElement('textarea');
        ta.value = command.value;
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
    }
    ElMessage.success(t('pro.node.copied'));
};

const submit = async () => {
    if (!formRef.value) return;
    const ok = await formRef.value.validate().catch(() => false);
    if (!ok) return;
    saving.value = true;
    try {
        if (editing.value) {
            await updateNode(props.kind, {
                id: form.id,
                name: form.name,
                groupID: form.groupID,
                description: form.description,
                ...(props.kind === 'panels'
                    ? { addr: form.addr, port: form.port, protocol: form.protocol, entrance: form.entrance, apiKey: form.apiKey }
                    : {}),
            });
            ElMessage.success(t('commons.msg.updateSuccess'));
        } else {
            const req: NodeCreate = { name: form.name, groupID: form.groupID, description: form.description };
            if (props.kind === 'nodes') {
                req.agentPort = form.agentPort;
                if (mode.value === 'ssh') req.ssh = { ...ssh };
            } else {
                Object.assign(req, { addr: form.addr, port: form.port, protocol: form.protocol, entrance: form.entrance, apiKey: form.apiKey });
            }
            const res = await createNode(props.kind, req);
            if (props.kind === 'nodes' && mode.value === 'command') {
                command.value = (await nodeJoinCommand(res.data.token)).data;
                cmdVisible.value = true;
                ElMessage.success(t('pro.node.created'));
            } else if (props.kind === 'nodes') {
                ElMessage.success(t('pro.node.sshStarted'));
            } else {
                ElMessage.success(t('commons.msg.createSuccess'));
            }
        }
        formVisible.value = false;
        search();
    } finally {
        saving.value = false;
    }
};

// --- delete / group / more ---
const doDelete = async (ids: number[]) => {
    const key = props.kind === 'nodes' ? 'pro.node.delConfirm' : 'pro.node.delPanelConfirm';
    await ElMessageBox.confirm(t(key, { n: ids.length }), t('pro.node.delete'), { type: 'warning' });
    await deleteNodes(props.kind, ids);
    ElMessage.success(t('commons.msg.deleteSuccess'));
    search();
};
const removeOne = (row: NodeRow) => doDelete([row.id]);
const removeSelected = () => doDelete(selected.value.map((r) => r.id));

const groupVisible = ref(false);
const targetGroup = ref(0);
const openGroup = () => {
    if (!selected.value.length) {
        ElMessage.warning(t('pro.node.selectFirst'));
        return;
    }
    targetGroup.value = groups.value.find((g) => g.isDefault)?.id || 0;
    groupVisible.value = true;
};
const applyGroup = async () => {
    await changeNodeGroup(props.kind, selected.value.map((r) => r.id), targetGroup.value);
    groupVisible.value = false;
    ElMessage.success(t('commons.msg.operationSuccess'));
    search();
};

const more = async (cmd: string, row: NodeRow) => {
    if (cmd === 'test') {
        await testNode(props.kind, row.id);
        ElMessage.success(t('pro.node.testOk'));
        search(true);
    } else if (cmd === 'restart') {
        await ElMessageBox.confirm(t('pro.node.restartConfirm'), t('pro.node.restart'), { type: 'warning' });
        await restartNode(row.id);
        ElMessage.success(t('pro.node.restartOk'));
    } else if (cmd === 'open') {
        const entrance = row.securityEntrance ? '/' + row.securityEntrance : '';
        window.open(`${row.protocol || 'http'}://${row.addr}:${row.port}${entrance}`, '_blank');
    }
};

watch(refresh, (v) => {
    if (timer) window.clearInterval(timer);
    timer = v > 0 ? window.setInterval(() => search(true), v * 1000) : undefined;
});

onMounted(async () => {
    groups.value = (await getGroupList('node')).data || [];
    search();
});
onBeforeUnmount(() => {
    if (timer) window.clearInterval(timer);
});
</script>

<style scoped>
.bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 8px;
    margin-bottom: 12px;
}
.right {
    display: flex;
    align-items: center;
    gap: 8px;
}
.name {
    display: flex;
    align-items: center;
    gap: 6px;
}
.edit {
    cursor: pointer;
    color: var(--el-color-primary);
}
.sub {
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.hint {
    display: block;
    width: 100%;
    color: var(--el-text-color-secondary);
    font-size: 12px;
    line-height: 18px;
}
.pager {
    display: flex;
    justify-content: flex-end;
    margin-top: 12px;
}
</style>
