import { afterEach, describe, expect, it } from 'vitest'
import { applyPublicBrand, brandMessages, brands, currentBrand } from '../index'
const original = { ...currentBrand }
afterEach(() => Object.assign(currentBrand, original))
describe('one shared campus website', () => {
 it('keeps canonical identities, protocols and download manifests independent', () => {
  expect(brands.muc.protocolScheme).toBe('muc')
  expect(brands.hubu.protocolScheme).toBe('hubu')
  expect(brands.hubu.manifestPath).not.toBe(brands.muc.manifestPath)
  expect(brands.muc.educationDomain).toBe('muc.edu.cn')
 })
 it('rejects a backend of the other school', () => {
  expect(() => applyPublicBrand({ id: 'hubu' })).toThrow('mismatch')
  expect(currentBrand.id).toBe('muc')
 })
 it('adapts presentation only and leaves the MUC source messages unchanged', () => {
  const source = { label: 'MUC AI', nested: { text: '中央民族大学 MUCODE' } }
  expect(brandMessages(source)).toEqual(source)
  Object.assign(currentBrand, brands.hubu)
  expect(brandMessages(source)).toEqual({ label: 'HUBU AI', nested: { text: '湖北大学 HUBU AI' } })
  expect(source.label).toBe('MUC AI')
  applyPublicBrand({ id: 'hubu', education_email_domain: 'test.hubu.example', download_base_url: 'http://localhost:18102/downloads' })
  expect(currentBrand.educationDomain).toBe('test.hubu.example')
  expect(currentBrand.downloadBase).toBe('http://localhost:18102/downloads')
 })
})
