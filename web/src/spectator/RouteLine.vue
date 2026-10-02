<script setup lang="ts">
import { computed } from 'vue'
import BadgeIcon from '../shared/components/BadgeIcon.vue'
import type { RouteLine } from './routeLine'

const props = defineProps<{ line: RouteLine }>()

// Stations sit at 5% + 10% * n across ten columns: start, eight gyms, League.
const filled = computed(() => `${props.line.fill * 90}%`)
</script>

<template>
  <div class="route-scroll">
    <ol class="route" :style="{ '--filled': filled }">
      <li class="stop is-town is-earned">
        <span class="dot" aria-hidden="true" />
        <span class="label"><b>{{ line.startTown }}</b><span class="sr-only">Start</span></span>
      </li>
      <li
        v-for="stop in line.stops"
        :key="stop.key"
        :class="['stop', { 'is-earned': stop.earned, 'is-next': stop.next }]"
        :style="{ '--gym': stop.color }"
        :aria-current="stop.next ? 'step' : undefined"
      >
        <span class="dot" aria-hidden="true">
          <BadgeIcon v-if="stop.earned" :name="stop.badge" :size="26" />
        </span>
        <span class="label">
          <b>{{ stop.town }}</b>
          {{ stop.badge }}
          <span class="sr-only">{{ stop.earned ? ', badge earned' : stop.next ? ', next badge' : ', badge not earned' }}</span>
          <em v-if="stop.next" aria-hidden="true">Next</em>
        </span>
      </li>
      <li
        :class="['stop', 'is-town', 'is-league', { 'is-earned': line.leagueEarned, 'is-next': line.leagueNext }]"
        :aria-current="line.leagueNext ? 'step' : undefined"
      >
        <span class="dot" aria-hidden="true" />
        <span class="label">
          <b>{{ line.leagueTown }}</b>
          Hall of Fame
          <span class="sr-only">{{ line.leagueEarned ? ', goal complete' : line.leagueNext ? ', next goal' : ', final goal' }}</span>
          <em v-if="line.leagueNext" aria-hidden="true">Next</em>
        </span>
      </li>
    </ol>
  </div>
</template>

<style scoped>
.route-scroll {
  width: 100%;
  max-width: 100%;
  min-width: 0;
  overflow-x: auto;
  overflow-y: hidden;
  padding-bottom: 0.5rem;
  overscroll-behavior-inline: contain;
  scrollbar-width: thin;
  -webkit-overflow-scrolling: touch;
}

.route {
  position: relative;
  display: grid;
  grid-template-columns: repeat(10, minmax(4.5rem, 1fr));
  min-width: 46rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.route::before,
.route::after {
  position: absolute;
  top: 0.8rem;
  left: 5%;
  height: 4px;
  content: '';
}

.route::before {
  right: 5%;
  background: var(--rule);
}

.route::after {
  width: var(--filled);
  background: var(--accent);
}

.stop {
  position: relative;
  z-index: 1;
  display: grid;
  justify-items: center;
  align-content: start;
  gap: 0.5rem;
  color: var(--dusk);
  font-size: 0.78rem;
  line-height: 1.25;
  text-align: center;
}

.dot {
  display: grid;
  width: 1.75rem;
  height: 1.75rem;
  place-items: center;
  border: 3px solid var(--rule);
  border-radius: 50%;
  background: var(--page);
}

.is-town .dot {
  width: 1.1rem;
  height: 1.1rem;
  margin: 0.325rem;
}

.is-earned .dot {
  border-color: var(--accent);
}

.is-town.is-earned .dot {
  background: var(--accent);
}

.is-league .dot {
  width: 1.45rem;
  height: 1.45rem;
  margin: 0.15rem;
}

.is-league.is-earned .dot {
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--accent) 16%, transparent);
}

.is-earned:not(.is-town) .dot {
  border-color: var(--gym);
  background: color-mix(in srgb, var(--gym) 22%, var(--page));
}

.is-next .dot {
  border-color: var(--accent);
  border-style: dashed;
}

.label {
  display: grid;
  gap: 1px;
}

.label b {
  color: var(--bone);
  font-weight: 600;
}

.label em {
  justify-self: center;
  margin-top: 2px;
  padding: 0 0.4rem;
  background: var(--accent);
  color: var(--page);
  font-size: 0.7rem;
  font-style: normal;
  font-weight: 700;
}
</style>
