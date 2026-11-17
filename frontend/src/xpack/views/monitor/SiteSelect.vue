<template>
    <el-select :model-value="modelValue || (all ? ALL : '')" style="width: 300px" @update:model-value="onChange">
        <template #prefix>{{ $t('pro.monitor.website') }}</template>
        <el-option v-if="all" :value="ALL" :label="$t('pro.monitor.allSites')" />
        <el-option v-for="s in sites" :key="s.alias" :value="s.alias" :label="labelOf(s)" />
    </el-select>
</template>

<script setup lang="ts">
import { MonSiteItem } from '@/xpack/api/monitor';

// el-select treats '' as "nothing selected", so "all websites" travels as a sentinel
const ALL = '__all';
defineProps<{ modelValue: string; sites: MonSiteItem[]; all?: boolean }>();
const emit = defineEmits(['update:modelValue']);
const onChange = (v: string) => emit('update:modelValue', v === ALL ? '' : v);

// on the master view aliases are prefixed with the node name ("node::alias");
// strip it for the label so the option shows "domain (node)" instead of the raw key
const labelOf = (s: MonSiteItem) => {
    const i = s.alias.indexOf('::');
    const node = i < 0 ? '' : s.alias.slice(0, i);
    const name = s.domain || (i < 0 ? s.alias : s.alias.slice(i + 2));
    return node ? `${name} (${node})` : name;
};
</script>
