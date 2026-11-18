<template>
    <div class="ov">
        <el-card shadow="never" class="top-card mb-3">
            <div class="top">
                <div class="left">
                    <el-select v-model="groupID" style="width: 150px" @change="filter">
                        <el-option :value="0" :label="$t('pro.node.allGroups')" />
                        <el-option
                            v-for="g in groups"
                            :key="g.id"
                            :value="g.id"
                            :label="g.isDefault && g.name.toLowerCase() === 'default' ? '默认' : g.name"
                        />
                    </el-select>
                    <el-input
                        v-model="info"
                        clearable
                        :placeholder="$t('pro.node.search')"
                        style="width: 220px"
                        @keyup.enter="filter"
                        @clear="filter"
                    >
                        <template #append>
                            <el-button :icon="Search" @click="filter" />
                        </template>
                    </el-input>
                </div>
                <div class="right">
                    <el-button :icon="Refresh" circle :loading="loading" @click="refreshAll" />
                </div>
            </div>
        </el-card>

        <el-row :gutter="12">
            <el-col :xs="24" :md="9" :lg="7" class="mb-3">
                <div v-loading="loading" class="node-list">
                    <el-card
                        v-for="n in nodes"
                        :key="n.name"
                        shadow="never"
                        class="node-card"
                        :class="{ active: n.name === current }"
                        @click="current = n.name"
                    >
                        <div class="node-head">
                            <div class="node-title">
                                <span class="nname">{{ n.isLocal ? globalStore.getMasterAlias() : n.name }}</span>
                                <el-tag size="small" :type="n.isLocal ? 'primary' : 'info'" effect="plain">
                                    {{ n.isLocal ? $t('pro.node.master') : $t('pro.node.childNode') }}
                                </el-tag>
                            </div>
                            <el-tag size="small" :type="statusType(n.status)" effect="light">
                                {{ $t('pro.node.status' + n.status) }}
                            </el-tag>
                        </div>

                        <div class="node-addr mono">{{ n.addr || '127.0.0.1' }}</div>

                        <div class="node-metrics">
                            <template v-if="n.cpuTotal">
                                <div class="metric-block">
                                    <div class="metric-top">
                                        <span class="metric-name">
                                            {{ $t('pro.node.cpu') }}
                                            <b :class="usageTone(n.cpuUsedPercent)">{{ clampPct(n.cpuUsedPercent).toFixed(1) }}%</b>
                                        </span>
                                        <span class="metric-cap">{{ n.cpuTotal }} {{ $t('pro.node.cores') }}</span>
                                    </div>
                                    <div class="metric-track">
                                        <div
                                            class="metric-fill"
                                            :class="usageTone(n.cpuUsedPercent)"
                                            :style="{ width: `${clampPct(n.cpuUsedPercent)}%` }"
                                        />
                                    </div>
                                </div>
                                <div class="metric-block">
                                    <div class="metric-top">
                                        <span class="metric-name">
                                            {{ $t('pro.node.memory') }}
                                            <b :class="usageTone(n.memoryUsedPercent)">{{ clampPct(n.memoryUsedPercent).toFixed(1) }}%</b>
                                        </span>
                                        <span class="metric-cap">{{ gb(n.memoryTotal) }}</span>
                                    </div>
                                    <div class="metric-track">
                                        <div
                                            class="metric-fill"
                                            :class="usageTone(n.memoryUsedPercent)"
                                            :style="{ width: `${clampPct(n.memoryUsedPercent)}%` }"
                                        />
                                    </div>
                                </div>
                            </template>
                            <div v-else class="metric-offline">{{ $t('pro.node.offlineHint') }}</div>
                        </div>

                        <div class="node-actions" @click.stop>
                            <el-link
                                type="primary"
                                :underline="false"
                                class="act-pill"
                                :disabled="n.status !== 'Healthy'"
                                @click="go(n, '/terminal')"
                            >
                                {{ $t('pro.node.terminal') }}
                            </el-link>
                            <span class="act-sep" />
                            <el-link
                                type="primary"
                                :underline="false"
                                class="act-pill"
                                :disabled="n.status !== 'Healthy'"
                                @click="go(n, '/logs')"
                            >
                                {{ $t('pro.node.log') }}
                            </el-link>
                            <template v-if="!n.isLocal">
                                <span class="act-sep" />
                                <el-link type="primary" :underline="false" class="act-pill" @click="restart(n)">
                                    {{ $t('pro.node.restart') }}
                                </el-link>
                            </template>
                        </div>
                    </el-card>
                    <el-empty v-if="!nodes.length && !loading" :image-size="60" :description="$t('pro.node.noData')" />
                </div>
                <div class="foot">
                    <span>{{ $t('pro.node.total', { n: total }) }}</span>
                    <el-pagination
                        v-model:current-page="page"
                        v-model:page-size="pageSize"
                        :total="total"
                        :page-sizes="[10, 20, 50]"
                        layout="sizes, prev, pager, next"
                        small
                        @change="search"
                    />
                </div>
            </el-col>

            <el-col :xs="24" :md="15" :lg="17">
                <el-card v-if="!current" shadow="never">
                    <el-empty :image-size="60" :description="$t('pro.node.noNodeSelected')" />
                </el-card>
                <div v-else-if="currentOnline" class="sections-grid">
                    <Section :title="$t('pro.node.agents')" :node="current" :token="token" :loader="loadAgents">
                        <el-table-column prop="name" :label="$t('pro.node.name')" min-width="130" show-overflow-tooltip />
                        <el-table-column prop="status" :label="$t('commons.table.status')" width="110" />
                    </Section>

                    <Section :title="$t('pro.node.apps')" :node="current" :token="token + (upgradable ? 1000 : 0)" :loader="loadApps">
                        <template #tools>
                            <el-checkbox v-model="upgradable" size="small">{{ $t('pro.node.upgradable') }}</el-checkbox>
                        </template>
                        <el-table-column prop="name" :label="$t('pro.node.name')" min-width="115" show-overflow-tooltip />
                        <el-table-column prop="status" :label="$t('commons.table.status')" width="95" />
                        <el-table-column prop="version" :label="$t('pro.node.version')" width="95" show-overflow-tooltip />
                        <el-table-column :label="$t('pro.node.installedAt')" width="150">
                            <template #default="{ row }">{{ fmt(row.createdAt) }}</template>
                        </el-table-column>
                        <el-table-column :label="$t('pro.node.operate')" width="110">
                            <template #default>
                                <el-link type="primary" :underline="false" @click="go(currentRow, '/logs')">
                                    {{ $t('pro.node.log') }}
                                </el-link>
                                <el-link type="primary" :underline="false" class="ml" @click="go(currentRow, '/apps/installed')">
                                    {{ $t('pro.node.moreAction') }}
                                </el-link>
                            </template>
                        </el-table-column>
                    </Section>

                    <Section :title="$t('pro.node.websites')" :node="current" :token="token" :loader="loadSites">
                        <el-table-column prop="primaryDomain" :label="$t('pro.node.name')" min-width="135" show-overflow-tooltip />
                        <el-table-column prop="type" :label="$t('commons.table.type')" width="95" />
                        <el-table-column prop="status" :label="$t('commons.table.status')" width="95" />
                        <el-table-column :label="$t('pro.node.expire')" width="115">
                            <template #default="{ row }">{{ (row.expireDate || '').slice(0, 10) }}</template>
                        </el-table-column>
                        <el-table-column :label="$t('pro.node.operate')" width="75">
                            <template #default>
                                <el-link type="primary" :underline="false" @click="go(currentRow, '/websites')">
                                    {{ $t('pro.node.backup') }}
                                </el-link>
                            </template>
                        </el-table-column>
                    </Section>

                    <Section :title="$t('pro.node.certs')" :node="current" :token="token" :loader="loadCerts">
                        <el-table-column prop="primaryDomain" :label="$t('pro.node.name')" min-width="140" show-overflow-tooltip />
                        <el-table-column prop="provider" :label="$t('commons.table.type')" width="115" />
                        <el-table-column :label="$t('pro.node.expire')" width="120">
                            <template #default="{ row }">{{ (row.expireDate || '').slice(0, 10) }}</template>
                        </el-table-column>
                    </Section>

                    <Section :title="$t('pro.node.databases')" :node="current" :token="token + dbToken" :loader="loadDbs">
                        <template #tools>
                            <el-select v-model="dbType" size="small" style="width: 110px" @change="onDbType">
                                <el-option value="mysql" label="MySQL" />
                                <el-option value="mariadb" label="MariaDB" />
                                <el-option value="postgresql" label="PostgreSQL" />
                                <el-option value="mongodb" label="MongoDB" />
                            </el-select>
                            <el-select v-model="dbServer" size="small" style="width: 115px" @change="dbToken++">
                                <el-option v-for="s in dbServers" :key="s" :value="s" :label="s" />
                            </el-select>
                        </template>
                        <el-table-column prop="name" :label="$t('pro.node.name')" min-width="115" show-overflow-tooltip />
                        <el-table-column prop="permission" :label="$t('pro.node.authUsers')" min-width="115" show-overflow-tooltip />
                        <el-table-column prop="description" :label="$t('pro.node.description')" min-width="110" show-overflow-tooltip />
                        <el-table-column :label="$t('pro.node.operate')" width="75">
                            <template #default>
                                <el-link type="primary" :underline="false" @click="go(currentRow, dbRoutePath)">
                                    {{ $t('pro.node.backup') }}
                                </el-link>
                            </template>
                        </el-table-column>
                    </Section>

                    <Section :title="$t('pro.node.crons')" :node="current" :token="token" :loader="loadCrons">
                        <el-table-column prop="name" :label="$t('pro.node.name')" min-width="135" show-overflow-tooltip />
                        <el-table-column prop="status" :label="$t('commons.table.status')" width="95" />
                        <el-table-column prop="lastRecordStatus" :label="$t('pro.node.lastRun')" min-width="115" show-overflow-tooltip />
                        <el-table-column :label="$t('pro.node.operate')" width="75">
                            <template #default>
                                <el-link type="primary" :underline="false" @click="go(currentRow, '/cronjobs')">
                                    {{ $t('pro.node.execute') }}
                                </el-link>
                            </template>
                        </el-table-column>
                    </Section>
                </div>
                <el-card v-else shadow="never">
                    <el-empty :image-size="60" :description="$t('pro.node.offlineHint')" />
                </el-card>
            </el-col>
        </el-row>
    </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { Refresh, Search } from '@element-plus/icons-vue';
import { useI18n } from 'vue-i18n';
import { GlobalStore, MenuStore } from '@/store';
import { getAgentSettingInfo } from '@/api/modules/setting';
import { routerToPathWithQuery } from '@/utils/router';
import { getGroupList } from '@/api/modules/group';
import { NodeRow, nodeQuery, restartNode, searchNodes } from '@/xpack/api/node';
import Section from './section.vue';

const { t } = useI18n();
const globalStore = GlobalStore();
const menuStore = MenuStore();

const groups = ref<{ id: number; name: string; isDefault: boolean }[]>([]);
const groupID = ref(0);
const info = ref('');
const nodes = ref<NodeRow[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);
const loading = ref(false);
const current = ref('');
const token = ref(0);
let timer: number | undefined;

const nodeName = (n: NodeRow) => (n.isLocal ? 'local' : n.name);
const currentRow = computed(() => nodes.value.find((n) => nodeName(n) === current.value) as NodeRow);
const currentOnline = computed(() => currentRow.value?.status === 'Healthy');
const statusType = (s: string) => (s === 'Healthy' ? 'success' : s === 'Installing' || s === 'Pending' ? 'warning' : 'danger');
const clampPct = (n?: number) => Math.min(100, Math.max(0, Number(n || 0)));
const usageTone = (n?: number) => {
    const p = clampPct(n);
    if (p >= 85) return 'is-danger';
    if (p >= 65) return 'is-warning';
    return 'is-success';
};
const gb = (n: number) => (n / 1024 / 1024 / 1024).toFixed(2) + ' GB';
const fmt = (s: string) => (s ? s.replace('T', ' ').slice(0, 19) : '-');

const search = async (silent: any = false) => {
    if (silent !== true) loading.value = true;
    try {
        const res = await searchNodes('nodes', {
            page: page.value,
            pageSize: pageSize.value,
            info: info.value,
            groupID: groupID.value || undefined,
        });
        // the list is keyed by name on the cards below; the master carries the name "local"
        nodes.value = (res.data.items || []).map((n) => ({ ...n, name: nodeName(n) }));
        total.value = res.data.total;
        if (!nodes.value.some((n) => n.name === current.value)) current.value = nodes.value[0]?.name || '';
    } finally {
        loading.value = false;
    }
};
const filter = () => {
    page.value = 1;
    search();
};
const refreshAll = () => {
    search();
    token.value++;
};

// jumping to a node's own page works by making it the operating node first
const go = async (n: NodeRow, path: string) => {
    const targetNode = nodeName(n);
    globalStore.currentNode = targetNode;
    globalStore.currentNodeAddr = n.addr;
    localStorage.removeItem('dashboardCache');
    menuStore.setMenuList([]);
    try {
        const res = await getAgentSettingInfo(targetNode);
        if (res?.data) {
            globalStore.defaultNetwork = res.data.defaultNetwork;
        }
    } catch {
        // ignore transient agent setting fetch errors
    }
    await routerToPathWithQuery(path, { uncached: 'true' });
};

const restart = async (n: NodeRow) => {
    await ElMessageBox.confirm(t('pro.node.restartConfirm'), t('pro.node.restart'), { type: 'warning' });
    await restartNode(n.id);
    ElMessage.success(t('pro.node.restartOk'));
};

// --- per-node resource loaders (all go through the master proxy) ---
const upgradable = ref(false);
const loadAgents = async (q: any) => {
    const res = await nodeQuery(current.value, '/ai/agents/search', { page: q.page, pageSize: q.pageSize, name: q.info });
    return { items: res.data?.items || [], total: res.data?.total || 0 };
};
const loadApps = async (q: any) => {
    const res = await nodeQuery(current.value, '/apps/installed/search', {
        page: q.page,
        pageSize: q.pageSize,
        name: q.info,
        update: upgradable.value,
        type: '',
        sync: false,
    });
    return { items: res.data?.items || [], total: res.data?.total || 0 };
};
const loadSites = async (q: any) => {
    const res = await nodeQuery(current.value, '/websites/search', {
        page: q.page,
        pageSize: q.pageSize,
        name: q.info,
        orderBy: 'created_at',
        order: 'null',
        websiteGroupId: 0,
    });
    return { items: res.data?.items || [], total: res.data?.total || 0 };
};
const loadCerts = async (q: any) => {
    const res = await nodeQuery(current.value, '/websites/ssl/search', {
        page: q.page,
        pageSize: q.pageSize,
        domain: q.info,
    });
    return { items: res.data?.items || [], total: res.data?.total || 0 };
};
const loadCrons = async (q: any) => {
    const res = await nodeQuery(current.value, '/cronjobs/search', {
        page: q.page,
        pageSize: q.pageSize,
        info: q.info,
        orderBy: 'createdAt',
        order: 'null',
    });
    return { items: res.data?.items || [], total: res.data?.total || 0 };
};

const dbType = ref('mysql');
const dbRoutePath = computed(() =>
    dbType.value === 'postgresql'
        ? '/databases/postgresql'
        : dbType.value === 'mongodb'
          ? '/databases/mongodb'
          : '/databases/mysql',
);
const dbServer = ref('');
const dbServers = ref<string[]>([]);
const dbToken = ref(0);
const loadDbServers = async () => {
    dbServers.value = [];
    dbServer.value = '';
    try {
        const res = await nodeQuery(current.value, '/databases/db/search', {
            type: dbType.value,
            page: 1,
            pageSize: 50,
            info: '',
            orderBy: 'createdAt',
            order: 'null',
        });
        dbServers.value = (res.data?.items || []).map((d: any) => d.name);
        dbServer.value = dbServers.value[0] || '';
    } catch {
        // no such database engine on this node
    }
    dbToken.value++;
};
const onDbType = () => loadDbServers();
const loadDbs = async (q: any) => {
    if (!dbServer.value) return { items: [], total: 0 };
    const url =
        dbType.value === 'postgresql'
            ? '/databases/pg/search'
            : dbType.value === 'mongodb'
              ? '/databases/mongodb/search'
              : '/databases/search';
    const res = await nodeQuery(current.value, url, {
        page: q.page,
        pageSize: q.pageSize,
        info: q.info,
        database: dbServer.value,
        type: dbType.value,
        orderBy: 'createdAt',
        order: 'null',
    });
    return { items: res.data?.items || [], total: res.data?.total || 0 };
};

watch(current, () => current.value && loadDbServers());

onMounted(async () => {
    groups.value = (await getGroupList('node')).data || [];
    await search();
    // node health changes on its own; keep the cards fresh without reloading the lists
    timer = window.setInterval(() => search(true), 15000);
});
onBeforeUnmount(() => {
    if (timer) window.clearInterval(timer);
});
</script>

<style scoped>
.top-card :deep(.el-card__body) {
    padding: 12px 16px;
}
.top {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
}
.left {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
}

.node-list {
    display: flex;
    flex-direction: column;
    gap: 10px;
}
.node-card {
    cursor: pointer;
    border-radius: 6px;
    border: 1px solid var(--el-border-color-lighter);
    transition: border-color 0.15s ease, background-color 0.15s ease, box-shadow 0.15s ease;
}
.node-card :deep(.el-card__body) {
    padding: 14px 16px;
}
.node-card:hover {
    border-color: var(--el-color-primary-light-5);
}
.node-card.active {
    border-color: var(--el-color-primary);
    box-shadow: 0 0 0 1px var(--el-color-primary) inset;
    background: var(--el-color-primary-light-9);
}

.node-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
}
.node-title {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
}
.nname {
    font-weight: 600;
    font-size: 14px;
    color: var(--el-text-color-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
.node-addr {
    margin-top: 4px;
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.node-metrics {
    margin-top: 12px;
    display: flex;
    flex-direction: column;
    gap: 8px;
}
.metric-block {
    display: flex;
    flex-direction: column;
    gap: 4px;
}
.metric-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 12px;
    font-variant-numeric: tabular-nums;
}
.metric-name {
    color: var(--el-text-color-regular);
}
.metric-name b {
    margin-left: 4px;
    font-weight: 600;
}
.metric-name b.is-success {
    color: var(--el-color-success);
}
.metric-name b.is-warning {
    color: var(--el-color-warning);
}
.metric-name b.is-danger {
    color: var(--el-color-danger);
}
.metric-cap {
    color: var(--el-text-color-secondary);
}
.metric-track {
    height: 5px;
    width: 100%;
    border-radius: 3px;
    background: var(--el-fill-color);
    overflow: hidden;
}
.metric-fill {
    height: 100%;
    border-radius: 3px;
    transition: width 0.3s ease;
}
.metric-fill.is-success {
    background: var(--el-color-success);
}
.metric-fill.is-warning {
    background: var(--el-color-warning);
}
.metric-fill.is-danger {
    background: var(--el-color-danger);
}
.metric-offline {
    padding: 8px 0;
    font-size: 12px;
    color: var(--el-text-color-secondary);
}

.node-actions {
    margin-top: 12px;
    padding-top: 10px;
    border-top: 1px dashed var(--el-border-color-lighter);
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 12px;
}
.act-pill {
    font-size: 12.5px;
    font-weight: 500;
}
.act-sep {
    width: 1px;
    height: 11px;
    background: var(--el-border-color-light);
}

.sections-grid {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 12px;
    align-items: stretch;
}
@media (min-width: 1440px) {
    .sections-grid {
        grid-template-columns: repeat(2, minmax(0, 1fr));
    }
}

.ml {
    margin-left: 10px;
}
.foot {
    margin-top: 10px;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
}
</style>
