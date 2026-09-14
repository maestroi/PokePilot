const BADGE_SPRITE_ROOT = 'https://raw.githubusercontent.com/PokeAPI/sprites/master/sprites/badges'

const KANTO_BADGES = [
  { name: 'Boulder Badge', slug: 'boulder', id: 1 },
  { name: 'Cascade Badge', slug: 'cascade', id: 2 },
  { name: 'Thunder Badge', slug: 'thunder', id: 3 },
  { name: 'Rainbow Badge', slug: 'rainbow', id: 4 },
  { name: 'Soul Badge', slug: 'soul', id: 5 },
  { name: 'Marsh Badge', slug: 'marsh', id: 6 },
  { name: 'Volcano Badge', slug: 'volcano', id: 7 },
  { name: 'Earth Badge', slug: 'earth', id: 8 }
] as const

const MILESTONE_ITEMS: { aliases: string[]; item: string }[] = [
  { aliases: ['ss-ticket', 's-s-ticket'], item: 'S.S. Ticket' },
  { aliases: ['dome-fossil'], item: 'Dome Fossil' },
  { aliases: ['helix-fossil'], item: 'Helix Fossil' },
  { aliases: ['old-amber'], item: 'Old Amber' },
  { aliases: ['bike-voucher'], item: 'Bike Voucher' },
  { aliases: ['bicycle'], item: 'Bicycle' },
  { aliases: ['coin-case'], item: 'Coin Case' },
  { aliases: ['itemfinder'], item: 'Itemfinder' },
  { aliases: ['silph-scope'], item: 'Silph Scope' },
  { aliases: ['poke-flute'], item: 'Poké Flute' },
  { aliases: ['lift-key'], item: 'Lift Key' },
  { aliases: ['card-key'], item: 'Card Key' },
  { aliases: ['secret-key'], item: 'Secret Key' },
  { aliases: ['old-rod'], item: 'Old Rod' },
  { aliases: ['good-rod'], item: 'Good Rod' },
  { aliases: ['super-rod'], item: 'Super Rod' }
]

const HM_BY_MOVE: Record<string, string> = {
  cut: 'HM01',
  fly: 'HM02',
  surf: 'HM03',
  strength: 'HM04',
  flash: 'HM05'
}

export type MilestoneVisualKind = 'badge' | 'item' | 'story'

function normalizeProgressName(value: string): string {
  return String(value || '')
    .trim()
    .toLowerCase()
    .replace(/[.'’]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

export function badgeName(value: string): string | null {
  const normalized = normalizeProgressName(value).replace(/-badge$/, '')
  const badge = KANTO_BADGES.find((entry) => normalized === entry.slug || normalized.includes(entry.slug))
  return badge?.name || null
}

export function badgeID(value: string): number | null {
  const name = badgeName(value)
  const badge = KANTO_BADGES.find((entry) => entry.name === name)
  return badge?.id || null
}

export function badgeSpriteUrl(value: string): string | null {
  const id = badgeID(value)
  return id ? `${BADGE_SPRITE_ROOT}/${id}.png` : null
}

export function milestoneBadgeName(value: string): string | null {
  return badgeName(value)
}

export function milestoneItemName(value: string): string | null {
  const normalized = normalizeProgressName(value)
  const machine = normalized.match(/(?:^|-)(tm|hm)-?(\d{1,2})(?:-|$)/)
  if (machine) return `${machine[1].toUpperCase()}${machine[2].padStart(2, '0')}`

  const known = MILESTONE_ITEMS.find((entry) => entry.aliases.some((alias) => normalized.includes(alias)))
  if (known) return known.item

  for (const [move, machineName] of Object.entries(HM_BY_MOVE)) {
    if (normalized.includes(move)) return machineName
  }
  return null
}

export function milestoneVisualKind(value: string): MilestoneVisualKind {
  if (milestoneBadgeName(value)) return 'badge'
  if (milestoneItemName(value)) return 'item'
  return 'story'
}
