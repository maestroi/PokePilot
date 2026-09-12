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
      title="Spectator"
      subtitle="Public read-only shell sharing the operator theme without inheriting operator capabilities."
      mode="public"
    >
      <NGrid cols="1 760:2" :x-gap="12" :y-gap="12">
        <NGi>
          <NCard title="Live run" size="small" class="poke-bay">
            <template #header-extra><NTag size="small" type="success" :bordered="false">Read-only</NTag></template>
            <p>The game frame, objective, party, progress, and public-safe run story will migrate into shared presentational components.</p>
          </NCard>
        </NGi>
        <NGi>
          <NCard title="Loading behavior" size="small" class="poke-bay">
            <ResourceState state="loading" :rows="3" />
          </NCard>
        </NGi>
      </NGrid>
    </AppShell>
  </UiRoot>
</template>
