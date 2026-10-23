// Wraps app.json. It adds the Firebase file (needed for push messages) only when it exists, so the app still builds
// without it. Locally, put google-services.json next to this file. On EAS, store it as a file environment variable named
// GOOGLE_SERVICES_JSON. See docs/push-notifications.md.
const fs = require('fs');

module.exports = ({ config }) => {
  const file = process.env.GOOGLE_SERVICES_JSON || './google-services.json';
  const hasFile = fs.existsSync(file);
  return {
    ...config,
    android: { ...config.android, ...(hasFile ? { googleServicesFile: file } : {}) },
  };
};
