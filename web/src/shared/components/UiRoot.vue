<script setup lang="ts">
import {
  darkTheme,
  NConfigProvider,
  NDialogProvider,
  NMessageProvider,
  NNotificationProvider
} from 'naive-ui'
import { pokePilotTheme } from '../theme'
</script>

<template>
  <NConfigProvider :theme="darkTheme" :theme-overrides="pokePilotTheme">
    <NDialogProvider>
      <NNotificationProvider>
        <NMessageProvider>
          <slot />
        </NMessageProvider>
      </NNotificationProvider>
    </NDialogProvider>
  </NConfigProvider>
</template>
