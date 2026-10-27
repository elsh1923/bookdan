# Monitoring and alerts

BookDan tells you about problems itself, and you should also watch it from outside.

## What the server alerts you about

An alert goes to a chat webhook and/or email. The same problem alerts at most once every 15 minutes (the next alert says how many were missed), and no more than 20 alerts are sent in any hour.

| Alert | When |
|---|---|
| A request panicked | any unhandled panic (the guest still gets a normal error answer) |
| An endpoint failed | an unexpected 500 on a route, named by its pattern (for example `POST /v1/bookings`) |
| Many server errors | 5 or more 5xx answers within 5 minutes |
| A refund could not be paid | the gateway refuses a refund (it is retried every 20 seconds until it works) |
| A daily job failed | completing stays, booking the pay-at-hotel commission |

An alert names the route and a request id, never a request body, a header, a token or an id from the URL. Payment mismatches, disputes, support requests and relocations already email the admins and `SUPPORT_EMAIL`.

## Settings

- `ALERT_WEBHOOK_URL`: a Slack, Discord or Microsoft Teams incoming-webhook URL (create one in the channel's integration settings).
- `ALERT_EMAIL`: comma separated addresses. Without it the admins in `ADMIN_EMAILS` are emailed (only when a real email provider is set).
- With neither, alerts are only written to the server log, and production logs a warning at start-up.

## Watch it from outside too

The server cannot tell you when it is down. Use a free uptime monitor (UptimeRobot, Better Stack, Pingdom) on:

- `https://<your server>/v1/healthz`: the process is up.
- `https://<your server>/v1/readyz`: the database answers (503 when it does not).

Set it to check every minute and to alert you by email or phone. Also turn on Render's own alerts for failed deploys.

## Still to do on your side

- Turn on Supabase backups and point-in-time recovery, and try a restore once.
- Choose a log place that keeps more than Render's short window if you want to look back (the logs are JSON, one line per request, with no secrets).
- Error tracking inside the phone apps (crashes) is not built; Expo's own crash reports (EAS) or Sentry can be added later.
