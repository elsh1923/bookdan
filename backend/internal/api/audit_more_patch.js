// One-off: small changes to existing test helpers so the new audit tests can reuse them.
const fs = require('fs');
function edit(file, pairs) {
  let s = fs.readFileSync(file, 'utf8');
  for (const [a, b] of pairs) {
    const n = s.split(a).length - 1;
    if (n !== 1) throw new Error(`${file}: expected 1 match, found ${n}: ${a.slice(0, 70)}`);
    s = s.replace(a, () => b);
  }
  fs.writeFileSync(file, s);
  console.log('edited', file);
}

// The fake Chapa can now fail refunds, so a retry can be tested.
edit('chapa_test.go', [
  ['\tverify   map[string]any\n', '\tverify   map[string]any\n\tfailRefunds bool // answer every refund with a server error, until switched off\n\trefundTries int\n'],
  [
    '\tmux.HandleFunc("/v2/refunds", func(w http.ResponseWriter, r *http.Request) {\n\t\tf.mu.Lock()\n\t\tdefer f.mu.Unlock()\n',
    '\tmux.HandleFunc("/v2/refunds", func(w http.ResponseWriter, r *http.Request) {\n\t\tf.mu.Lock()\n\t\tdefer f.mu.Unlock()\n\t\tif f.failRefunds {\n\t\t\tf.refundTries++\n\t\t\thttp.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)\n\t\t\treturn\n\t\t}\n',
  ],
]);

// auditServer can also change the providers (for example an email provider that always fails).
edit('audit_search_test.go', [
  ['func auditServer(t *testing.T, change func(*config.Config)) (restore func()) {', 'func auditServer(t *testing.T, change func(*config.Config), mods ...func(*api.Providers)) (restore func()) {'],
  ['\tproviders.OTP, providers.EmailReal = api.RoutedSender(mailer, mailer), true\n\tapp2, err := api.New(context.Background(), cfg, pool, keys, providers)', '\tproviders.OTP, providers.EmailReal = api.RoutedSender(mailer, mailer), true\n\tfor _, m := range mods {\n\t\tm(&providers)\n\t}\n\tapp2, err := api.New(context.Background(), cfg, pool, keys, providers)'],
]);
