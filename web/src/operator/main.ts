import { createApp } from 'vue'
import App from './App.vue'
import '../shared/base.css'

const missingFrameTitle = 'Last recorded frame unavailable.'

function runFrameImage(target: EventTarget | null): HTMLImageElement | null {
  if (!(target instanceof HTMLImageElement)) return null
  const source = target.getAttribute('src') || ''
  return source.startsWith('/frame?run=') ? target : null
}

function markRunFrameMissing(image: HTMLImageElement): void {
  const host = image.parentElement
  if (!host) return
  host.dataset.runFrameMissing = 'true'
  host.title = missingFrameTitle
}

function clearRunFrameMissing(image: HTMLImageElement): void {
  const host = image.parentElement
  if (!host) return
  delete host.dataset.runFrameMissing
  if (host.title === missingFrameTitle) host.removeAttribute('title')
}

const frameFallbackStyle = document.createElement('style')
frameFallbackStyle.textContent = `
  [data-run-frame-missing="true"] > img[src^="/frame?run="] {
    visibility: hidden;
  }
  [data-run-frame-missing="true"]::after {
    content: "NO FRAME";
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 9px;
    letter-spacing: 0.06em;
    color: var(--poke-dim);
    pointer-events: none;
  }
`
document.head.append(frameFallbackStyle)

document.addEventListener('error', (event) => {
  const image = runFrameImage(event.target)
  if (image) markRunFrameMissing(image)
}, true)

document.addEventListener('load', (event) => {
  const image = runFrameImage(event.target)
  if (image) clearRunFrameMissing(image)
}, true)

createApp(App).mount('#app')
