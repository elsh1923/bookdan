import { Component, useEffect, useMemo, useRef, type ReactNode } from 'react';
import { Platform, View } from 'react-native';
import type { SearchItem } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { EmptyState } from '@bookdan/ui';
import { price } from './format';
import { useDisplay } from './display';

// The map is a small web page (Leaflet with OpenStreetMap tiles, so no map key is needed) shown in a WebView. The WebView is a native
// module: a phone running an older build of the app does not have it, so it is loaded carefully and the screen falls back to a message.
let WebViewImpl: any = null;
try {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  WebViewImpl = require('react-native-webview').WebView;
} catch {
  WebViewImpl = null;
}

const SITE = 'https://bookdan.onrender.com'; // the page's address, so the map tiles are asked for with a proper Referer

interface Pin { id: string; name: string; lat: number; lng: number; label: string }

/** The HTML of the map: one price label per hotel (tap to pick it), and the guest's own position when known. */
export function mapHtml(pins: Pin[], me: { lat: number; lng: number } | null): string {
  const data = JSON.stringify({ pins, me }).replace(/</g, '\\u003c');
  return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,maximum-scale=1">
<link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css">
<link rel="stylesheet" href="https://unpkg.com/leaflet.markercluster@1.5.3/dist/MarkerCluster.css">
<style>html,body,#map{height:100%;margin:0;font-family:sans-serif}
.pin{display:inline-block;width:max-content;background:#2563eb;color:#fff;font:700 12px sans-serif;padding:5px 9px;border-radius:14px;white-space:nowrap;border:2px solid #fff;box-shadow:0 1px 4px rgba(0,0,0,.45);transform:translate(-50%,-100%)}
.cl{width:38px;height:38px;border-radius:50%;background:#0f1b33;color:#fff;font:700 14px sans-serif;display:flex;align-items:center;justify-content:center;border:3px solid #fff;box-shadow:0 1px 4px rgba(0,0,0,.45)}
.me{width:14px;height:14px;border-radius:50%;background:#1d4ed8;border:3px solid #fff;box-shadow:0 0 0 2px #1d4ed8}</style></head>
<body><div id="map"></div>
<script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js"></script>
<script src="https://unpkg.com/leaflet.markercluster@1.5.3/dist/leaflet.markercluster.js"></script>
<script>
var data = ${data};
var map = L.map('map').setView([9.01, 38.76], 12);
L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', { maxZoom: 19, attribution: '&copy; OpenStreetMap contributors' }).addTo(map);
function send(id) {
  var m = JSON.stringify({ id: id });
  if (window.ReactNativeWebView) window.ReactNativeWebView.postMessage(m); else parent.postMessage(m, '*');
}
var pts = [];
var group = L.markerClusterGroup({ showCoverageOnHover: false, maxClusterRadius: 48, iconCreateFunction: function (c) { return L.divIcon({ className: '', html: '<div class="cl">' + c.getChildCount() + '</div>', iconSize: [38, 38] }); } });
map.addLayer(group);
data.pins.forEach(function (p) {
  var d = document.createElement('div'); d.className = 'pin'; d.textContent = p.label;
  group.addLayer(L.marker([p.lat, p.lng], { icon: L.divIcon({ className: '', html: d, iconSize: [0, 0] }) }).on('click', function () { send(p.id); }));
  pts.push([p.lat, p.lng]);
});
if (data.me) {
  var m = document.createElement('div'); m.className = 'me';
  L.marker([data.me.lat, data.me.lng], { icon: L.divIcon({ className: '', html: m, iconSize: [14, 14] }) }).addTo(map);
  pts.push([data.me.lat, data.me.lng]);
}
if (pts.length > 1) map.fitBounds(pts, { padding: [48, 48], maxZoom: 15 }); else if (pts.length === 1) map.setView(pts[0], 14);
</script></body></html>`;
}

class Boundary extends Component<{ fallback: ReactNode; children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  render() {
    return this.state.failed ? this.props.fallback : this.props.children;
  }
}

/** The hotels of a search as price labels on a map. onPick gets the id of the hotel the guest tapped. */
export function HotelMap({ items, me, onPick }: { items: SearchItem[]; me: { lat: number; lng: number } | null; onPick: (id: string) => void }) {
  const { t } = useTranslation();
  const shown = useDisplay((st) => st.version); // the price labels follow the display currency
  const pins = useMemo<Pin[]>(
    () => items.filter((i) => i.lat && i.lng).map((i) => ({ id: i.id, name: i.name, lat: i.lat, lng: i.lng, label: price(i.from_price_santim, i.currency) })),
    [items, shown],
  );
  const html = useMemo(() => mapHtml(pins, me), [pins, me]);
  const frame = useRef<HTMLIFrameElement | null>(null);

  // On the web (used for testing) the page runs in an iframe and tells us what was tapped with postMessage.
  useEffect(() => {
    if (Platform.OS !== 'web') return;
    const listen = (e: MessageEvent) => {
      if (frame.current && e.source === frame.current.contentWindow) {
        try { onPick(JSON.parse(String(e.data)).id); } catch { /* not ours */ }
      }
    };
    window.addEventListener('message', listen);
    return () => window.removeEventListener('message', listen);
  }, [onPick]);

  const missing = <EmptyState icon="🗺️" title={t('results.map')} body={t('results.map_update')} />;
  if (Platform.OS === 'web') {
    return (
      <View style={{ flex: 1 }}>
        <iframe ref={frame} srcDoc={html} title="map" style={{ border: 0, width: '100%', height: '100%' }} />
      </View>
    );
  }
  if (!WebViewImpl) return missing;
  return (
    <View style={{ flex: 1 }}>
      <Boundary fallback={missing}>
        <WebViewImpl
          originWhitelist={['*']}
          source={{ html, baseUrl: SITE }}
          javaScriptEnabled
          domStorageEnabled
          onMessage={(e: { nativeEvent: { data: string } }) => {
            try { onPick(JSON.parse(e.nativeEvent.data).id); } catch { /* not ours */ }
          }}
          style={{ flex: 1 }}
        />
      </Boundary>
    </View>
  );
}
