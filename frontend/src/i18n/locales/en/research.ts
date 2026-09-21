export default {
  research: {
    title: 'Research Discount',
    description: 'Submit proof of your research identity and receive a balance discount once approved',

    intro: {
      title: 'About Research Discount',
      rule1: 'The research discount is available to faculty and researchers and adds credit to your account balance.',
      rule2: 'Briefly describe your research identity in the application and upload supporting documents.',
      rule3: 'Applications are reviewed manually by administrators; track progress under "My Applications" below.',
      rule4: 'Once approved, the reward amount is automatically credited to your account balance.'
    },

    form: {
      title: 'Submit Application',
      descriptionLabel: 'Application Description',
      descriptionPlaceholder:
        'Briefly introduce yourself and your research, e.g. faculty member at a university computer science department who needs LLM APIs for a research project…',
      descriptionRequired: 'Please enter the application description',
      descriptionTooLong: 'Description cannot exceed {max} characters',
      charCount: '{count}/{max}',
      attachmentsLabel: 'Supporting Documents (up to {max})',
      attachmentsHint: 'JPG / PNG / WebP / PDF supported, max 5MB per file',
      uploadAreaText: 'Click to select or drag files here',
      uploading: 'Uploading {percent}%',
      uploadFailed: 'Failed to upload "{name}"',
      removeAttachment: 'Remove',
      fileTooLarge: '"{name}" exceeds the 5MB size limit',
      fileTypeUnsupported: '"{name}" is not a supported file type',
      tooManyFiles: 'Up to {max} attachments allowed',
      submit: 'Submit Application',
      submitting: 'Submitting...',
      attachmentsUploading: 'Attachments are still uploading, please wait…',
      submitSuccess: 'Application submitted, waiting for review',
      submitFailed: 'Failed to submit application, please try again later'
    },

    list: {
      title: 'My Applications',
      empty: 'No applications yet',
      emptyHint: 'Your review progress will appear here after your first submission',
      submittedAt: 'Submitted at: {time}',
      reviewedAt: 'Reviewed at: {time}',
      descriptionLabel: 'Application Description',
      reviewNotesLabel: 'Review Notes',
      noReviewNotes: 'No review notes',
      rewardGranted: 'Balance credited',
      attachmentsLabel: 'Attachments ({count})',
      download: 'Download',
      downloading: 'Downloading…',
      downloadFailed: 'Failed to download attachment',
      expand: 'Expand',
      collapse: 'Collapse'
    },

    status: {
      pending: 'Pending',
      approved: 'Approved',
      rejected: 'Rejected'
    },

    loadFailed: 'Failed to load applications'
  }
}
