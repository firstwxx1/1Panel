import { jumpToPath } from './router';
import router from '@/routers';
import { GlobalStore } from '@/store';
import { vllmAvailable } from '@/views/ai/model/deployment';

const jumpIfRouteExists = (target: string) => {
    const resolved = router.resolve(target);
    if (
        !resolved.matched.length ||
        resolved.name === '404' ||
        resolved.path.startsWith('/error') ||
        resolved.matched.some((m) => m.path.includes('pathMatch'))
    ) {
        return false;
    }
    jumpToPath(router, target);
    return true;
};

export const jumpToInstall = (type: string, key: string) => {
    const globalStore = GlobalStore();
    switch (type) {
        case 'php':
        case 'node':
        case 'java':
        case 'go':
        case 'python':
        case 'dotnet':
            jumpToPath(router, '/websites/runtimes/' + type);
            return true;
    }
    switch (key) {
        case 'openclaw':
            jumpToPath(router, '/ai/agents/agent');
            return true;
        case 'copaw':
            router.push({
                path: '/ai/agents/agent',
                query: {
                    uncached: 'true',
                    open: 'create',
                    agentType: 'copaw',
                },
            });
            return true;
        case 'hermes-agent':
            router.push({
                path: '/ai/agents/agent',
                query: {
                    uncached: 'true',
                    open: 'create',
                    agentType: 'hermes-agent',
                },
            });
            return true;
        case 'ollama':
            router.push({ path: '/ai/model/local', query: { engine: 'ollama', uncached: 'true', open: 'create' } });
            return true;
        case 'vllm':
            if (globalStore.isProductPro && vllmAvailable) {
                router.push({
                    path: '/ai/model/local',
                    query: {
                        tab: 'vllm',
                        uncached: 'true',
                        open: 'create',
                    },
                });
                return true;
            }
            return false;
        case 'mysql-cluster':
            return jumpIfRouteExists('/xpack/cluster/mysql');
        case 'redis-cluster':
            return jumpIfRouteExists('/xpack/cluster/redis');
        case 'postgresql-cluster':
            return jumpIfRouteExists('/xpack/cluster/postgres');
    }
    return false;
};
