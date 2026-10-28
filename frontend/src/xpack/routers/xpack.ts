import { Layout } from '@/routers/constant';
import '@/xpack/lang';
import '@/xpack/lang/node';
import '@/xpack/lang/monitor';
import '@/xpack/lang/sync';
import '@/xpack/lang/alert';
import '@/xpack/lang/tamper';
import '@/xpack/lang/uisetting';

const xpackRouter = {
    sort: 3.5,
    path: '/xpack',
    name: 'Xpack-Menu',
    component: Layout,
    redirect: '/xpack/waf',
    meta: {
        icon: 'p-xpack',
        title: 'pro.menu',
    },
    children: [
        {
            path: '/xpack/waf',
            name: 'ProWaf',
            component: () => import('@/xpack/views/waf/index.vue'),
            meta: {
                icon: 'p-waf-1',
                title: 'pro.waf.title',
            },
        },
        {
            path: '/xpack/monitor',
            name: 'ProMonitor',
            component: () => import('@/xpack/views/monitor/index.vue'),
            meta: {
                icon: 'p-monitor-menu',
                title: 'pro.monitor.title',
            },
        },
        {
            path: '/xpack/sync',
            name: 'ProSync',
            component: () => import('@/xpack/views/sync/index.vue'),
            meta: {
                icon: 'p-exchange-menu',
                title: 'pro.sync.title',
            },
        },
        {
            path: '/xpack/tamper',
            name: 'ProTamper',
            component: () => import('@/xpack/views/tamper/index.vue'),
            meta: {
                icon: 'p-waf-1',
                title: 'pro.tamper.title',
            },
        },
        {
            path: '/xpack/alert',
            name: 'ProAlert',
            component: () => import('@/xpack/views/alert/index.vue'),
            meta: {
                icon: 'p-monitor-menu',
                title: 'pro.alert.title',
            },
        },
        {
            path: '/xpack/uisetting',
            name: 'ProUiSetting',
            component: () => import('@/xpack/views/uisetting/index.vue'),
            meta: {
                icon: 'p-xpack',
                title: 'pro.uisetting.title',
            },
        },
        {
            path: '/xpack/node',
            name: 'NodeDashboard',
            component: () => import('@/xpack/views/node/index.vue'),
            meta: {
                icon: 'p-node-menu',
                title: 'pro.node.title',
            },
        },
    ],
};

export default xpackRouter;
