import { createApp, defineComponent, h } from 'vue'
import App from './App.vue'
import AppShell from '../shared/components/AppShell.vue'
import '../shared/base.css'

const ReplayRoot = defineComponent({
  name: 'ReplayRoot',
  setup() {
    return () => h(AppShell, {
      eyebrow: 'Public archive',
      title: 'Replay library',
      subtitle: '',
      mode: 'public',
      showIntro: false
    }, {
      default: () => h(App)
    })
  }
})

createApp(ReplayRoot).mount('#app')
