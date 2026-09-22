import { currentBrand } from '@/brand'
import hubuLogo from '@/assets/hubu/site-logo.png'
/** Retain the correct school identity when a configured logo fails to load. */
export function useDefaultLogo(event: Event) {
  const image = event.target as HTMLImageElement
  const fallback = currentBrand.id === 'hubu' ? hubuLogo : '/logo.svg'
  if (image.getAttribute('src') !== fallback) image.src = fallback
}
