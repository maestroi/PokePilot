export const MAX_THEME_ASSET_BYTES = 4 * 1024 * 1024

const ALLOWED_THEME_ASSET_EXTENSIONS = new Set(['.png', '.webp'])
const ALLOWED_THEME_ASSET_MIME_TYPES = new Set(['image/png', 'image/webp'])

export interface ThemeAssetFileDescriptor {
  name: string
  size: number
  type?: string
}

function decodedPath(value: string): string | null {
  try {
    return decodeURIComponent(value)
  } catch {
    return null
  }
}

function extension(path: string): string {
  const dot = path.lastIndexOf('.')
  return dot >= 0 ? path.slice(dot).toLowerCase() : ''
}

function unsafeSegments(path: string): boolean {
  return path.split('/').some((segment) => segment === '.' || segment === '..')
}

export function validateThemeAssetReference(reference: string): string | null {
  if (!reference || reference !== reference.trim()) return 'must be a non-empty local asset reference'
  if (reference.length > 512) return 'must be at most 512 characters'
  if (/[\\\u0000-\u001f\u007f]/.test(reference)) return 'contains unsafe path characters'

  const rawPath = reference.split(/[?#]/, 1)[0]
  const path = decodedPath(rawPath)
  if (!path) return 'contains invalid URL encoding'
  if (!path.startsWith('/theme-assets/')) return 'must stay under /theme-assets/'
  if (unsafeSegments(path)) return 'must not contain path traversal'
  if (!ALLOWED_THEME_ASSET_EXTENSIONS.has(extension(path))) {
    return 'must reference a PNG or WebP image'
  }
  return null
}

export function validateThemeAssetFile(file: ThemeAssetFileDescriptor): string[] {
  const errors: string[] = []
  const name = decodedPath(file.name)

  if (!name || !name.trim() || name !== name.trim()) {
    errors.push('file name is invalid')
  } else {
    if (name.startsWith('/') || /[\\\u0000-\u001f\u007f]/.test(name) || unsafeSegments(name)) {
      errors.push('file name must be a safe relative path')
    }
    if (!ALLOWED_THEME_ASSET_EXTENSIONS.has(extension(name))) {
      errors.push('file type must be PNG or WebP')
    }
  }

  if (!Number.isFinite(file.size) || file.size <= 0 || file.size > MAX_THEME_ASSET_BYTES) {
    errors.push(`file size must be between 1 byte and ${MAX_THEME_ASSET_BYTES} bytes`)
  }
  if (file.type && !ALLOWED_THEME_ASSET_MIME_TYPES.has(file.type.toLowerCase())) {
    errors.push('file MIME type must be image/png or image/webp')
  }

  return errors
}
