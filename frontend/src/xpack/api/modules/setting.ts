import http from '@/api';

export const searchXSetting = () => http.get<any>('/core/xpack/settings/search');

export const getXProxyDocker = () => http.get<any>('/core/xpack/settings/proxy');

export const updateXSettingByKey = (key: string, value: string) =>
    http.post('/core/xpack/settings/update', { key, value });

export const uploadXImage = (kind: string, file: File) => {
    const fd = new FormData();
    fd.append('kind', kind);
    fd.append('file', file);
    return http.upload('/core/xpack/settings/upload', fd);
};
