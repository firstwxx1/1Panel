<template>
    <el-select :model-value="modelValue || (all ? ALL : '')" style="width: 300px" @update:model-value="onChange">
        <template #prefix>{{ $t('pro.monitor.website') }}</template>
        <el-option v-if="all" :value="ALL" :label="$t('pro.monitor.allSites')" />
        <el-option v-for="s in sites" :key="s.alias" :value="s.alias" :label="s.domain || s.alias" />
    </el-select>
</template>

<script setup lang="ts">
import { MonSiteItem } from '@/xpack/api/monitor';

// el-select treats '' as "nothing selected", so "all websites" travels as a sentinel
const ALL = '__all';
defineProps<{ modelValue: string; sites: MonSiteItem[]; all?: boolean }>();
const emit = defineEmits(['update:modelValue']);
const onChange = (v: string) => emit('update:modelValue', v === ALL ? '' : v);
</script>
