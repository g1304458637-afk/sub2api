/**
 * Presentation Pricing admin i18n.
 *
 * Boundary: this console only affects what users see on the "Models & Pricing"
 * page; it never affects real billing. Copy repeats this deliberately.
 */
export default {
  presentationPricing: {
    searchPlaceholder: 'Search models...',
    headerNote:
      'Affects the user-facing "Models & Pricing" page only — never actual API billing',
    warning:
      'Changing display prices does NOT change real billing. Actual charges remain governed by the existing billing system (channel pricing, group multipliers, subscription rules).',
    loadFailed: 'Failed to load presentation pricing',
    plazaUnavailable:
      'Model Plaza is disabled — no models to configure. Enable Model Plaza in system settings first.',
    empty: 'No models to configure',
    noSearchResult: 'No matching models',
    saved: 'Display price saved — effective immediately on the user-facing pricing page',
    cleared: 'Restored defaults (custom display price cleared)',
    saveFailed: 'Failed to save display price',
    disabledBadge: 'Disabled',
    edit: 'Edit',
    billingManagedByBilling: 'Determined by the existing billing system',
    perImage: 'image',
    perRequest: 'request',
    table: {
      model: 'Model',
      displayPrice: 'Display price (in/out)',
      source: 'Source',
      officialPrice: 'Official reference',
      billingNote: 'Actual billing'
    },
    source: {
      manual: 'Custom override',
      official: 'Official fallback',
      billing: 'Billing fallback',
      none: 'Price not published'
    },
    editor: {
      displaySection: 'User-facing display price',
      displaySectionHint:
        'Shown on the user-facing "Models & Pricing" page once saved; disable to fall back',
      enableOverride: 'Enable custom display price',
      inputPrice: 'Input',
      outputPrice: 'Output',
      cacheWritePrice: 'Cache write (5m)',
      cacheWrite1hPrice: 'Cache write (1h)',
      cacheReadPrice: 'Cache read',
      perRequestPrice: 'Per-request price',
      remark: 'Remark (admins only)',
      referenceSection: 'Reference (read-only)',
      resolvedDisplay: 'What users see',
      restoreDefault: 'Restore default',
      save: 'Save display price',
      saveHint: 'Changing display prices does not change real billing',
      priceRequired: 'At least one price is required'
    }
  }
}
