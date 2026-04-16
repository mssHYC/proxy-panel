import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const routes = [
  { path: '/login', name: 'Login', component: () => import('../views/Login.vue') },
  {
    path: '/',
    component: () => import('../components/Layout.vue'),
    children: [
      { path: '', name: 'Dashboard', component: () => import('../views/Dashboard.vue') },
      { path: 'users', name: 'Users', component: () => import('../views/Users.vue') },
      { path: 'nodes', name: 'Nodes', component: () => import('../views/Nodes.vue') },
      { path: 'traffic', name: 'Traffic', component: () => import('../views/Traffic.vue') },
      { path: 'settings', name: 'Settings', component: () => import('../views/Settings.vue') },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to) => {
  const auth = useAuthStore()
  if (to.name !== 'Login' && !auth.isLoggedIn()) {
    return { name: 'Login' }
  }
})

export default router
