<script setup lang="ts">
import AppShell from '../shared/components/AppShell.vue'
import type { AppNavItem } from '../shared/types'
import OperationsView from './OperationsView.vue'

const navigation: AppNavItem[] = [
  { name: 'Live', href: '#live' },
  { name: 'Runs', href: '#runs' },
  { name: 'Failures', href: '#failures' },
  { name: 'Analytics', href: '#analytics' },
  { name: 'Operations', href: '#operations', current: true },
  { name: 'Tools', href: '#tools' }
]
</script>

<template>
  <AppShell
    eyebrow="System"
    title="Operations"
    subtitle="Fleet health, workers, active attempts, queue/leases, and recent outcomes update independently without reloading the page."
    mode="private"
    :navigation="navigation"
  >
    <OperationsView />
  </AppShell>
</template>
