import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import './style.css'

// element-plus 的样式由 unplugin-vue-components ElementPlusResolver 按需注入；
// 命令式 API（ElMessage / ElMessageBox / ElNotification / ElLoading）由
// unplugin-auto-import 自动 import，对应的样式 SCSS 也由 resolver 一并解析，
// 因此 main.ts 不再需要全局 import 'element-plus/dist/index.css'。

// 仅注册实际用到的 icons，避免把 @element-plus/icons-vue 全量打进包。
// 新增模板用到的 icon 时，请同步把组件名追加到下面的 import + 注册。
import {
  Connection,
  CopyDocument,
  DataLine,
  Delete,
  Document,
  Edit,
  Link,
  Odometer,
  Plus,
  Refresh,
  Setting,
  SwitchButton,
  Upload,
  User,
} from '@element-plus/icons-vue'

const app = createApp(App)

const icons = {
  Connection,
  CopyDocument,
  DataLine,
  Delete,
  Document,
  Edit,
  Link,
  Odometer,
  Plus,
  Refresh,
  Setting,
  SwitchButton,
  Upload,
  User,
}
for (const [name, comp] of Object.entries(icons)) {
  app.component(name, comp)
}

app.use(createPinia())
app.use(router)
app.mount('#app')
