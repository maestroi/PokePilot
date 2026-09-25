const GEN1_RENDER_REVISION = '0cd19d3'
const FRAME_SIZE = 16

const imageCache = new Map<string, Promise<HTMLImageElement>>()

export function gen1SpriteURL(asset: string | null | undefined): string {
  if (!asset) return ''
  return `/gen1/red/sprites/${asset}.png?rev=${GEN1_RENDER_REVISION}`
}

export async function loadGen1Sprite(asset: string): Promise<HTMLImageElement> {
  const url = gen1SpriteURL(asset)
  let pending = imageCache.get(url)
  if (!pending) {
    pending = new Promise<HTMLImageElement>((resolve, reject) => {
      const image = new Image()
      image.decoding = 'async'
      image.onload = () => resolve(image)
      image.onerror = () => {
        imageCache.delete(url)
        reject(new Error(`failed to load ${url}`))
      }
      image.src = url
    })
    imageCache.set(url, pending)
  }
  return pending
}

function facingRow(facing: string | null | undefined, image: HTMLImageElement): number {
  if (image.naturalHeight < FRAME_SIZE * 3) return 0
  if (facing === 'UP') return 1
  if (facing === 'LEFT' || facing === 'RIGHT') return 2
  return 0
}

export function drawGen1Sprite(
  ctx: CanvasRenderingContext2D,
  image: HTMLImageElement,
  fieldX: number,
  fieldY: number,
  fieldCellPixels: number,
  facing?: string
): void {
  const sourceSize = Math.min(FRAME_SIZE, image.naturalWidth, image.naturalHeight)
  const sourceY = Math.min(
    facingRow(facing, image) * FRAME_SIZE,
    Math.max(0, image.naturalHeight - sourceSize)
  )
  const destSize = fieldCellPixels
  const left = fieldX * fieldCellPixels + (fieldCellPixels - destSize) / 2
  const top = fieldY * fieldCellPixels + (fieldCellPixels - destSize) / 2

  ctx.save()
  ctx.imageSmoothingEnabled = false
  if (facing === 'RIGHT' && image.naturalHeight >= FRAME_SIZE * 3) {
    ctx.translate(left + destSize, top)
    ctx.scale(-1, 1)
    ctx.drawImage(image, 0, sourceY, sourceSize, sourceSize, 0, 0, destSize, destSize)
  } else {
    ctx.drawImage(image, 0, sourceY, sourceSize, sourceSize, left, top, destSize, destSize)
  }
  ctx.restore()
}
