export default {
  research: {
    title: 'Research Review',
    description: 'Review research discount applications submitted by users',

    filter: {
      all: 'All Statuses',
      pending: 'Pending',
      approved: 'Approved',
      rejected: 'Rejected'
    },

    columns: {
      applicant: 'Applicant',
      description: 'Description',
      attachments: 'Attachments',
      createdAt: 'Submitted At',
      status: 'Status',
      actions: 'Actions'
    },

    status: {
      pending: 'Pending',
      approved: 'Approved',
      rejected: 'Rejected'
    },

    attachmentCount: '{count} file(s)',
    viewDetail: 'Detail',
    applicantFallback: 'User {id}',

    detail: {
      title: 'Application Detail',
      applicant: 'Applicant',
      submittedAt: 'Submitted At',
      reviewedAt: 'Reviewed At',
      notReviewed: 'Not reviewed',
      descriptionLabel: 'Application Description',
      attachmentsLabel: 'Supporting Documents',
      noAttachments: 'No attachments',
      preview: 'Preview',
      download: 'Download',
      downloadFailed: 'Failed to download attachment',
      previewFailed: 'Failed to preview attachment',
      reviewNotes: 'Review Notes',
      noReviewNotes: 'No review notes',
      rewardAmount: 'Reward Amount',
      approve: 'Approve',
      reject: 'Reject',
      close: 'Close'
    },

    approve: {
      title: 'Approve Application',
      applicantHint: 'The reward amount will be credited to {name}\'s account balance',
      amountLabel: 'Reward Amount ($)',
      amountInvalid: 'Please enter a valid amount (greater than 0)',
      notesLabel: 'Review Notes (optional)',
      notesPlaceholder: 'Optional, visible to the user',
      confirm: 'Approve',
      submitting: 'Submitting...',
      success: 'Application approved and balance credited',
      failed: 'Failed to approve application'
    },

    reject: {
      title: 'Reject Application',
      notesLabel: 'Rejection Reason',
      notesPlaceholder: 'Required, visible to the user',
      notesRequired: 'Please enter the rejection reason',
      confirm: 'Reject',
      submitting: 'Submitting...',
      success: 'Application rejected',
      failed: 'Failed to reject application'
    },

    list: {
      loadFailed: 'Failed to load applications',
      empty: 'No applications',
      emptyHint: 'No applications match the current filter'
    }
  }
}
