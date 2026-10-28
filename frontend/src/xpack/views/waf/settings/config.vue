<template>
    <el-form label-width="180px" style="max-width: 620px">
        <el-form-item :label="$t('pro.waf.logEnable')">
            <el-switch v-model="log.state" active-value="on" inactive-value="off" />
        </el-form-item>
        <el-form-item :label="$t('pro.waf.logRetention')">
            <el-input-number v-model="log.maxDay" :min="1" :max="3650" controls-position="right" />
        </el-form-item>
        <el-form-item :label="$t('pro.waf.logMaxSize')">
            <el-input-number v-model="log.maxSize" :min="1" :max="1024" controls-position="right" />
        </el-form-item>
        <el-form-item :label="$t('pro.waf.secret')">
            <el-input v-model="secret" show-password />
        </el-form-item>
        <el-form-item>
            <el-button type="primary" @click="save">{{ $t('pro.waf.configSave') }}</el-button>
        </el-form-item>
    </el-form>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { ElMessage } from 'element-plus';
import { useI18n } from 'vue-i18n';
import { updateWafGlobal } from '@/xpack/api/waf';

const props = defineProps<{ config: Record<string, any> }>();
const emit = defineEmits<{ reload: [] }>();
const { t } = useI18n();

const log = computed(() => props.config.log || { state: 'on', maxDay: 180, maxSize: 1 });
const secret = ref(props.config.waf?.secret || '');

const save = async () => {
    await updateWafGlobal({
        log: log.value,
        waf: { ...(props.config.waf || {}), secret: secret.value },
    });
    ElMessage.success(t('pro.waf.saveSuccess'));
    emit('reload');
};
</script>
