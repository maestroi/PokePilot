export interface ProgressItem {
  name?: string
  quantity?: number
}

export interface ProgressPlayer {
  bag_used?: number
  bag_capacity?: number
  bag?: ProgressItem[]
  dex_owned?: number
  dex_seen?: number
  dex_total?: number
  milestones?: string[]
}

export function bagMeter(player?: ProgressPlayer | null): string | null {
  const capacity = Number(player?.bag_capacity || 0)
  if (capacity <= 0) return null
  return `${Number(player?.bag_used || 0)}/${capacity}`
}

export function dexMeter(player?: ProgressPlayer | null): string | null {
  const total = Number(player?.dex_total || 0)
  if (total <= 0) return null
  return `${Number(player?.dex_owned || 0)}/${total}`
}

export function dexDetail(player?: ProgressPlayer | null): string | null {
  const total = Number(player?.dex_total || 0)
  if (total <= 0) return null
  return `${Number(player?.dex_owned || 0)} owned / ${Number(player?.dex_seen || 0)} seen / ${total}`
}

export function bagItemsLabel(player?: ProgressPlayer | null): string {
  const items = player?.bag ?? []
  if (!items.length) return 'Empty'
  return items
    .map((item) => `${item.name || 'item'} ×${Number(item.quantity || 0)}`)
    .join(', ')
}

export function milestonesLabel(player?: ProgressPlayer | null): string {
  const beats = player?.milestones ?? []
  if (!beats.length) return 'No major milestones yet'
  return beats.join(', ')
}
