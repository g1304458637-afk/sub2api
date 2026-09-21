export default {
  rewardGrants: {
    title: 'Reward Grants',
    description: 'Ledger of system rewards (student verification, etc.). Read-only',

    filters: {
      userId: 'User ID',
      userIdPlaceholder: 'Exact user ID',
      campaign: 'Campaign',
      campaignPlaceholder: 'Exact campaign identifier',
      sourceType: 'Source',
      sourceTypeAll: 'All sources'
    },

    sourceType: {
      student_verification: 'Student Verification'
    },

    columns: {
      time: 'Granted At',
      user: 'User',
      amount: 'Amount',
      sourceType: 'Source Type',
      campaign: 'Campaign',
      grantedBy: 'Granted By',
      idempotencyKey: 'Idempotency Key'
    },

    grantedBySystem: 'System',
    empty: 'No reward grants',
    emptyHint: 'No reward grants match the current filters',
    loadFailed: 'Failed to load reward grants'
  }
}
