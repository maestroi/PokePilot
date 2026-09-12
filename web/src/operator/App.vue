<script setup lang="ts">
import { NCard, NGi, NGrid, NTag } from 'naive-ui'
import AppShell from '../shared/components/AppShell.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import UiRoot from '../shared/components/UiRoot.vue'
</script>

<template>
  <UiRoot>
    <AppShell
      eyebrow="PokéPilot"
      title="Operator console"
      subtitle="Vue migration shell — existing operator UI remains authoritative while views move over incrementally."
      mode="private"
    >
      <NGrid cols="1 760:2 1180:3" :x-gap="12" :y-gap="12">
        <NGi>
          <NCard title="Operations" size="small" class="poke-bay">
            <template #header-extra><NTag size="small" type="info" :bordered="false">First migration target</NTag></template>
            <p>Workers, active attempts, queue, health, and model routing will move here before the live game workspace.</p>
          </NCard>
        </NGi>
        <NGi>
          <NCard title="Runs" size="small" class="poke-bay">
            <p>Archive filtering, pagination, cleanup, and run selection can migrate without changing backend contracts.</p>
          </NCard>
        </NGi>
        <NGi>
          <NCard title="Shared resource state" size="small" class="poke-bay">
            <ResourceState state="stale" message="Example: keep the last dashboard snapshot visible while a background refresh retries." />
          </NCard>
        </NGi>
      </NGrid>
    </AppShell>
  </UiRoot>
</template>
