const POKEMON_SPRITE_ROOT = 'https://raw.githubusercontent.com/PokeAPI/sprites/master/sprites/pokemon/versions/generation-i/red-blue'
const ITEM_SPRITE_ROOT = 'https://raw.githubusercontent.com/PokeAPI/sprites/master/sprites/items'

const GEN_ONE_SPECIES = [
  'bulbasaur',
  'ivysaur',
  'venusaur',
  'charmander',
  'charmeleon',
  'charizard',
  'squirtle',
  'wartortle',
  'blastoise',
  'caterpie',
  'metapod',
  'butterfree',
  'weedle',
  'kakuna',
  'beedrill',
  'pidgey',
  'pidgeotto',
  'pidgeot',
  'rattata',
  'raticate',
  'spearow',
  'fearow',
  'ekans',
  'arbok',
  'pikachu',
  'raichu',
  'sandshrew',
  'sandslash',
  'nidoran-f',
  'nidorina',
  'nidoqueen',
  'nidoran-m',
  'nidorino',
  'nidoking',
  'clefairy',
  'clefable',
  'vulpix',
  'ninetales',
  'jigglypuff',
  'wigglytuff',
  'zubat',
  'golbat',
  'oddish',
  'gloom',
  'vileplume',
  'paras',
  'parasect',
  'venonat',
  'venomoth',
  'diglett',
  'dugtrio',
  'meowth',
  'persian',
  'psyduck',
  'golduck',
  'mankey',
  'primeape',
  'growlithe',
  'arcanine',
  'poliwag',
  'poliwhirl',
  'poliwrath',
  'abra',
  'kadabra',
  'alakazam',
  'machop',
  'machoke',
  'machamp',
  'bellsprout',
  'weepinbell',
  'victreebel',
  'tentacool',
  'tentacruel',
  'geodude',
  'graveler',
  'golem',
  'ponyta',
  'rapidash',
  'slowpoke',
  'slowbro',
  'magnemite',
  'magneton',
  'farfetchd',
  'doduo',
  'dodrio',
  'seel',
  'dewgong',
  'grimer',
  'muk',
  'shellder',
  'cloyster',
  'gastly',
  'haunter',
  'gengar',
  'onix',
  'drowzee',
  'hypno',
  'krabby',
  'kingler',
  'voltorb',
  'electrode',
  'exeggcute',
  'exeggutor',
  'cubone',
  'marowak',
  'hitmonlee',
  'hitmonchan',
  'lickitung',
  'koffing',
  'weezing',
  'rhyhorn',
  'rhydon',
  'chansey',
  'tangela',
  'kangaskhan',
  'horsea',
  'seadra',
  'goldeen',
  'seaking',
  'staryu',
  'starmie',
  'mr-mime',
  'scyther',
  'jynx',
  'electabuzz',
  'magmar',
  'pinsir',
  'tauros',
  'magikarp',
  'gyarados',
  'lapras',
  'ditto',
  'eevee',
  'vaporeon',
  'jolteon',
  'flareon',
  'porygon',
  'omanyte',
  'omastar',
  'kabuto',
  'kabutops',
  'aerodactyl',
  'snorlax',
  'articuno',
  'zapdos',
  'moltres',
  'dratini',
  'dragonair',
  'dragonite',
  'mewtwo',
  'mew',
] as const

const SPECIES_ALIASES: Record<string, string> = {
  'farfetch-d': 'farfetchd',
  'mr-mime': 'mr-mime',
  'mrmime': 'mr-mime',
  'nidoran-female': 'nidoran-f',
  'nidoran-f': 'nidoran-f',
  'nidoran-male': 'nidoran-m',
  'nidoran-m': 'nidoran-m'
}

const ITEM_ALIASES: Record<string, string> = {
  'pokeball': 'poke-ball',
  'poke-ball': 'poke-ball',
  'great-ball': 'great-ball',
  'ultra-ball': 'ultra-ball',
  'master-ball': 'master-ball',
  'potion': 'potion',
  'super-potion': 'super-potion',
  'hyper-potion': 'hyper-potion',
  'max-potion': 'max-potion',
  'full-restore': 'full-restore',
  'antidote': 'antidote',
  'burn-heal': 'burn-heal',
  'ice-heal': 'ice-heal',
  'awakening': 'awakening',
  'paralyze-heal': 'paralyze-heal',
  'full-heal': 'full-heal',
  'revive': 'revive',
  'max-revive': 'max-revive',
  'escape-rope': 'escape-rope',
  'repel': 'repel',
  'super-repel': 'super-repel',
  'max-repel': 'max-repel',
  'rare-candy': 'rare-candy',
  'nugget': 'nugget',
  'dome-fossil': 'dome-fossil',
  'helix-fossil': 'helix-fossil',
  'old-amber': 'old-amber',
  'ss-ticket': 'ss-ticket',
  's-s-ticket': 'ss-ticket',
  'bike-voucher': 'bike-voucher',
  'bicycle': 'bicycle',
  'coin-case': 'coin-case',
  'itemfinder': 'itemfinder',
  'silph-scope': 'silph-scope',
  'poke-flute': 'poke-flute',
  'lift-key': 'lift-key',
  'card-key': 'card-key',
  'secret-key': 'secret-key',
  'old-rod': 'old-rod',
  'good-rod': 'good-rod',
  'super-rod': 'super-rod',
  'exp-all': 'exp-share'
}

export type ItemVisualKind = 'item' | 'ball' | 'tm' | 'hm' | 'key'

export function normalizeAssetName(value: string): string {
  return String(value || '')
    .trim()
    .toLowerCase()
    .replace(/[.'’]/g, '')
    .replace(/[♀]/g, '-female')
    .replace(/[♂]/g, '-male')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

export function pokemonDexNumber(name: string): number | null {
  const normalized = normalizeAssetName(name)
  const species = SPECIES_ALIASES[normalized] || normalized
  const index = GEN_ONE_SPECIES.indexOf(species as (typeof GEN_ONE_SPECIES)[number])
  return index >= 0 ? index + 1 : null
}

export function pokemonSpriteUrl(name: string): string | null {
  const dex = pokemonDexNumber(name)
  return dex ? `${POKEMON_SPRITE_ROOT}/${dex}.png` : null
}

export function itemVisualKind(name: string): ItemVisualKind {
  const normalized = normalizeAssetName(name)
  if (/^hm-?\d+$/i.test(normalized)) return 'hm'
  if (/^tm-?\d+$/i.test(normalized)) return 'tm'
  if (normalized.includes('ball')) return 'ball'
  if (/(ticket|key|voucher|scope|flute|rod|fossil|amber|bicycle|coin-case|itemfinder)/.test(normalized)) return 'key'
  return 'item'
}

export function itemSpriteUrl(name: string): string | null {
  const normalized = normalizeAssetName(name)
  if (itemVisualKind(name) === 'tm' || itemVisualKind(name) === 'hm') return null
  const slug = ITEM_ALIASES[normalized]
  return slug ? `${ITEM_SPRITE_ROOT}/${slug}.png` : null
}

export function itemToken(name: string): string {
  const normalized = normalizeAssetName(name)
  const machine = normalized.match(/^(tm|hm)-?(\d+)$/)
  if (machine) return `${machine[1].toUpperCase()}${machine[2].padStart(2, '0')}`
  if (itemVisualKind(name) === 'ball') return '●'
  if (itemVisualKind(name) === 'key') return '◆'
  return '✦'
}

export function itemDisplayName(name: string): string {
  const raw = String(name || '').trim()
  const normalized = normalizeAssetName(raw)
  const machine = normalized.match(/^(tm|hm)-?(\d+)$/)
  if (machine) return `${machine[1].toUpperCase()}${machine[2].padStart(2, '0')}`
  if (normalized === 'ss-ticket' || normalized === 's-s-ticket') return 'S.S. Ticket'
  if (normalized === 'pokeball' || normalized === 'poke-ball') return 'Poké Ball'
  if (!raw) return 'Unknown item'
  return raw
    .split(/\s+/)
    .map((word) => word ? word[0].toUpperCase() + word.slice(1) : word)
    .join(' ')
}
