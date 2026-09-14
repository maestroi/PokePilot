const POKEAPI_RAW_PREFIX = 'https://raw.githubusercontent.com/PokeAPI/sprites/master/'
const POKEPILOT_ASSET_PREFIX = '/poke-assets/'

/**
 * Keep artwork requests same-origin in the browser. The data layer can retain
 * canonical PokeAPI URLs while pokeui proxies the small whitelisted sprite
 * subtree, so spectator CSP stays strict and reverse proxies cannot block the
 * artwork as a third-party image.
 */
export function browserPokemonAssetUrl(source: string | null): string | null {
  if (!source) return null
  if (!source.startsWith(POKEAPI_RAW_PREFIX)) return source
  return POKEPILOT_ASSET_PREFIX + source.slice(POKEAPI_RAW_PREFIX.length)
}
