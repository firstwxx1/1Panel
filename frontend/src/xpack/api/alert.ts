import http from '@/api';

export interface AlertRule {
    enabled: boolean;
    threshold: number;
}

export interface AlertTelegram {
    enabled: boolean;
    token: string;
    chatId: string;
    proxy: string;
}

export interface AlertConfig {
    telegram: AlertTelegram;
    interval: number;
    cooldownMin: number;
    rules: Record<string, AlertRule>;
}

export interface AlertRecord {
    time: string;
    rule: string;
    node: string;
    message: string;
    sent: boolean;
    error: string;
}

const base = '/core/xpack/alert';

export const getAlertConfig = () => http.get<AlertConfig>(`${base}/config`);
export const saveAlertConfig = (req: AlertConfig) => http.post(`${base}/config`, req);
export const testTelegram = (req: AlertTelegram) => http.post(`${base}/telegram/test`, req);
export const telegramChats = (req: AlertTelegram) =>
    http.post<{ id: string; name: string }[]>(`${base}/telegram/chats`, req);
export const listAlertRecords = () => http.get<AlertRecord[]>(`${base}/records`);
export const clearAlertRecords = () => http.post(`${base}/records/clear`, {});
