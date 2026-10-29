// BookDan Hotel Portal. Plain JavaScript, no libraries. Every piece of text from the server is put on the page with textContent,
// never as HTML, so a guest's or hotel's name can never run code.
(function () {
  'use strict';

  var API = '/v1';
  var AMENITIES = [['generator', 'Generator'], ['hot_water', 'Hot water'], ['wifi', 'Wi-Fi'], ['parking', 'Parking'], ['pay_at_hotel', 'Pay at hotel'], ['ac', 'Air conditioning'], ['pool', 'Swimming pool'], ['restaurant', 'Restaurant'], ['gym', 'Gym'], ['spa', 'Spa'], ['airport_shuttle', 'Airport shuttle'], ['laundry', 'Laundry'], ['front_desk_24h', '24-hour front desk'], ['water_tank', '24-hour water']];
  var TYPES = [['hotel', 'Hotel'], ['guesthouse', 'Guesthouse'], ['lodge', 'Lodge'], ['resort', 'Resort']];
  var STATUSES = [['draft', 'Draft'], ['active', 'Published'], ['hidden', 'Hidden']];

  // role is 'owner' when the signed-in person owns at least one hotel here, otherwise 'staff' (a receptionist: bookings and availability only).
  var state = { token: null, refresh: null, contact: '', role: 'staff', hotels: [], hotelId: '' };
  function store(k, v) { try { if (v === null) sessionStorage.removeItem(k); else sessionStorage.setItem(k, v); } catch (e) { /* private mode */ } }
  function stored(k) { try { return sessionStorage.getItem(k); } catch (e) { return null; } }
  state.token = stored('hp-token'); state.refresh = stored('hp-refresh'); state.contact = stored('hp-contact') || ''; state.hotelId = stored('hp-hotel') || '';
  var app = document.getElementById('app');

  // A stable id for this browser, the way the phone app has one: the server ties sign-in codes and sessions to it.
  function deviceId() {
    var id = null;
    try { id = localStorage.getItem('hp-device'); } catch (e) { /* ignore */ }
    if (!id) {
      id = (window.crypto && crypto.randomUUID) ? crypto.randomUUID() : 'web-' + Date.now() + '-' + Math.random().toString(16).slice(2);
      try { localStorage.setItem('hp-device', id); } catch (e) { /* ignore */ }
    }
    return id;
  }

  // ---- tiny DOM helper: h('div', {class: 'x', onclick: fn}, 'text', child, ...)
  function h(tag, props) {
    var el = document.createElement(tag);
    for (var k in (props || {})) {
      var v = props[k];
      if (v === undefined || v === null || v === false) continue;
      if (k === 'class') el.className = v;
      else if (k.slice(0, 2) === 'on') el.addEventListener(k.slice(2), v);
      else if (k === 'style') el.style.cssText = v; // set through the style object: the page's content policy blocks inline style attributes
      else if (k === 'value') el.value = v;
      else if (k === 'checked') el.checked = !!v;
      else el.setAttribute(k, v === true ? '' : v);
    }
    for (var i = 2; i < arguments.length; i++) add(el, arguments[i]);
    return el;
  }
  function add(el, c) {
    if (c === undefined || c === null || c === false) return;
    if (Array.isArray(c)) c.forEach(function (x) { add(el, x); });
    else el.appendChild(typeof c === 'string' || typeof c === 'number' ? document.createTextNode(String(c)) : c);
  }
  function clear(el) { while (el.firstChild) el.removeChild(el.firstChild); }
  function msg(kind, text) { return h('div', { class: 'msg ' + kind, role: kind === 'err' ? 'alert' : 'status' }, text); }
  function birr(santim) { return santim == null ? '' : String(Math.round(santim / 100)); }
  function etb(santim) { return 'ETB ' + (santim / 100).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }); }
  function go(hash) { if (location.hash === hash) render(); else location.hash = hash; }

  // ---- server calls. The access token is short-lived: on a 401 it is traded once for a new one with the refresh token.
  var refreshing = null;
  function refreshTokens() {
    if (!state.refresh) return Promise.resolve(false);
    refreshing = refreshing || fetch(API + '/auth/refresh', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-App': 'partner', 'X-Device-Id': deviceId() }, body: JSON.stringify({ refresh_token: state.refresh }) })
      .then(function (res) { return res.ok ? res.json() : null; }, function () { return null; })
      .then(function (json) {
        refreshing = null;
        if (!json || !json.token) return false;
        state.token = json.token; state.refresh = json.refresh_token || state.refresh;
        store('hp-token', state.token); store('hp-refresh', state.refresh);
        return true;
      });
    return refreshing;
  }
  function call(method, path, body, retried) {
    var opts = { method: method, headers: { 'X-App': 'partner', 'X-Device-Id': deviceId() } };
    if (state.token) opts.headers.Authorization = 'Bearer ' + state.token;
    if (body instanceof FormData) opts.body = body;
    else if (body !== undefined) { opts.headers['Content-Type'] = 'application/json'; opts.body = JSON.stringify(body); }
    return fetch(API + path, opts).then(function (res) {
      return res.json().catch(function () { return {}; }).then(function (json) {
        if (res.ok) return json;
        if (res.status === 401 && state.token && !retried) {
          return refreshTokens().then(function (ok) {
            if (ok) return call(method, path, body, true);
            signOut();
            throw new Error('Please sign in again');
          });
        }
        var e = new Error((json.error && json.error.message) || 'Something went wrong (' + res.status + ')');
        e.status = res.status;
        e.code = json.error && json.error.code;
        throw e;
      });
    }, function () { throw new Error('No connection to the server'); });
  }
  function signOut() {
    var rt = state.refresh;
    if (rt) fetch(API + '/auth/logout', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-App': 'partner', 'X-Device-Id': deviceId() }, body: JSON.stringify({ refresh_token: rt }) }).catch(function () { /* best effort */ });
    state.token = state.refresh = null; state.hotels = []; state.role = 'staff';
    store('hp-token', null); store('hp-refresh', null);
    render();
  }

  // ---- sign in: the same email or phone code as the hotel app
  function loginView() {
    var box = h('div', { class: 'card login' });
    var contact = h('input', { type: 'text', placeholder: 'name@example.com or +2519…', autocomplete: 'username' });
    var code = h('input', { type: 'text', inputmode: 'numeric', maxlength: '6', placeholder: '6-digit code', autocomplete: 'one-time-code' });
    var codeField = h('div', { class: 'field', style: 'display:none' }, h('label', {}, 'Code we sent you'), code);
    var btn = h('button', { type: 'submit' }, 'Send me a code');
    var note = h('div', {});
    var stage = 1;
    var form = h('form', { onsubmit: function (e) {
      e.preventDefault(); clear(note); btn.disabled = true;
      var c = contact.value.trim(), ident = { type: c.indexOf('@') >= 0 ? 'email' : 'phone', value: c };
      if (stage === 1) {
        call('POST', '/auth/otp/request', ident).then(function () {
          stage = 2; btn.disabled = false; codeField.style.display = ''; btn.textContent = 'Sign in'; code.focus();
          add(note, msg('ok', 'We sent a code to ' + c + '. It is valid for a few minutes.'));
        }, function (err) { btn.disabled = false; add(note, msg('err', err.message)); });
      } else {
        call('POST', '/auth/otp/verify', { type: ident.type, value: ident.value, code: code.value.trim() }).then(function (res) {
          state.token = res.token; state.refresh = res.refresh_token; state.contact = c;
          store('hp-token', state.token); store('hp-refresh', state.refresh); store('hp-contact', c);
          go('#/');
        }, function (err) { btn.disabled = false; add(note, msg('err', err.message)); });
      }
    } }, h('div', { class: 'field' }, h('label', {}, 'Your email or phone'), contact), codeField, note, h('div', { class: 'field' }, btn));
    add(box, h('h1', {}, 'BookDan Hotel Portal'));
    add(box, h('p', { class: 'soft' }, 'Run your hotel: bookings, rooms and prices, availability, photos and your team. Sign in with the email or phone your hotel is registered with.'));
    add(box, form);
    add(box, h('p', { class: 'muted', style: 'font-size:13px;margin-top:16px' }, 'Not registered yet? Ask BookDan to add your hotel, or apply at /apply.'));
    return h('div', {}, box);
  }

  // ---- shell: sidebar + content
  var NAV = [
    ['#/', 'Home', false], ['#/bookings', 'Bookings', false], ['#/calendar', 'Availability', false],
    ['#/hotel', 'My hotel', true], ['#/rooms', 'Rooms & prices', true], ['#/photos', 'Photos', true], ['#/team', 'Team', true], ['#/alerts', 'Booking alerts', true],
    ['#/earnings', 'Earnings', true], ['#/statements', 'Statements', true], ['#/reviews', 'Reviews', true], ['#/help', 'Help from BookDan', false]
  ];
  function currentHotel() {
    for (var i = 0; i < state.hotels.length; i++) if (state.hotels[i].id === state.hotelId) return state.hotels[i];
    return state.hotels[0] || null;
  }
  function shell(active, title, content) {
    var items = NAV.filter(function (n) { return !n[2] || state.role === 'owner'; });
    var nav = h('nav', { class: 'side' });
    var SECTIONS = { '#/': 'Today', '#/hotel': 'Manage', '#/earnings': 'Money', '#/help': 'Support' };
    items.forEach(function (n) {
      if (SECTIONS[n[0]]) add(nav, h('h4', {}, SECTIONS[n[0]]));
      add(nav, h('a', { href: n[0], class: n[0] === active ? 'on' : '' }, n[1]));
    });
    var switcher = null;
    if (state.hotels.length > 1) {
      var sel = h('select', { 'aria-label': 'Hotel', onchange: function () { state.hotelId = sel.value; store('hp-hotel', sel.value); render(); } },
        state.hotels.map(function (x) { return h('option', { value: x.id }, x.name); }));
      sel.value = currentHotel() ? currentHotel().id : '';
      switcher = h('div', { class: 'switch' }, h('label', {}, 'Hotel'), sel);
    }
    return h('div', { class: 'layout' },
      h('aside', {}, h('div', { class: 'brand' }, h('b', {}, 'BookDan', h('span', {}, 'Hotel Portal'))), switcher, nav,
        h('div', { class: 'who' }, h('div', { class: 'soft' }, state.contact), h('div', {}, h('span', { class: 'chip' }, state.role === 'owner' ? 'Owner' : 'Reception')),
          h('button', { class: 'ghost', onclick: signOut }, 'Sign out'))),
      h('main', {}, h('h1', {}, title), content));
  }
  function loading() { return h('p', { class: 'muted' }, 'Loading…'); }
  function fail(holder, err) { clear(holder); add(holder, msg('err', err.message)); }

  // Addis Ababa calendar date, the one hotels and guests use for "today".
  function todayISO(offset) {
    var d = new Date(Date.now() + (offset || 0) * 86400000);
    return new Intl.DateTimeFormat('en-CA', { timeZone: 'Africa/Addis_Ababa', year: 'numeric', month: '2-digit', day: '2-digit' }).format(d);
  }
  function day(iso) { return new Date(iso + (iso.length === 10 ? 'T12:00:00' : '')).toLocaleDateString(undefined, { day: 'numeric', month: 'short' }); }

  // ---- home
  function homeView() {
    var holder = h('div', {}, loading());
    var jobs = [call('GET', '/partner/bookings?limit=200')];
    if (state.role === 'owner') jobs.push(call('GET', '/partner/earnings').catch(function () { return null; }));
    Promise.all(jobs).then(function (res) {
      var items = res[0].items, earn = res[1];
      var today = todayISO();
      var waiting = items.filter(function (b) { return b.status === 'PAID_AWAITING_HOTEL'; });
      var arriving = items.filter(function (b) { return b.status === 'CONFIRMED' && b.check_in === today; });
      var inHouse = items.filter(function (b) { return b.status === 'CHECKED_IN'; });
      var leaving = items.filter(function (b) { return b.status === 'CHECKED_IN' && b.check_out === today; });
      clear(holder);
      var tiles = [['Waiting for your reply', waiting.length, waiting.length ? 'warn' : ''], ['Arriving today', arriving.length, ''], ['Staying now', inHouse.length, ''], ['Leaving today', leaving.length, '']];
      if (earn) {
        var due = 0; earn.hotels.forEach(function (x) { due += x.to_pay_santim; });
        tiles.push(['BookDan owes you', etb(due), 'ok']);
      }
      add(holder, h('div', { class: 'tiles' }, tiles.map(function (t) { return h('div', { class: 'tile ' + t[2] }, h('div', { class: 'num' }, String(t[1])), h('div', { class: 'soft' }, t[0])); })));
      if (waiting.length) {
        add(holder, h('div', { class: 'card' }, h('div', { class: 'row between' }, h('h2', {}, 'Answer these now'), h('a', { class: 'btn secondary', href: '#/bookings' }, 'Open bookings')),
          h('p', { class: 'soft' }, 'A guest has paid and is waiting. If nobody answers in time the guest is refunded and the room is released.'),
          bookingTable(waiting, load)));
      }
      var next = items.filter(function (b) { return b.status === 'CONFIRMED' && b.check_in >= today; }).sort(function (a, b) { return a.check_in < b.check_in ? -1 : 1; }).slice(0, 8);
      add(holder, h('div', { class: 'card' }, h('h2', {}, 'Next arrivals'), next.length ? bookingTable(next, load) : h('p', { class: 'muted' }, 'No confirmed bookings coming up yet.')));
      if (state.role === 'owner') {
        var h0 = currentHotel();
        if (h0 && h0.status !== 'active') add(holder, h('div', { class: 'card' }, msg('warn', h0.name + ' is ' + (h0.status === 'draft' ? 'a draft' : 'hidden') + ', so guests cannot see it. BookDan publishes hotels once the rooms, prices and business details are ready. Contact BookDan support from the Help page.')));
      }
      function load() { render(); }
    }, function (err) { fail(holder, err); });
    return holder;
  }

  // ---- bookings
  var DECLINE = [['fully_booked', 'Fully booked'], ['room_unavailable', 'Room not available'], ['maintenance', 'Maintenance'], ['other', 'Other']];
  var CANCEL = [['overbooked', 'Overbooked'], ['maintenance', 'Maintenance'], ['emergency', 'Emergency'], ['other', 'Other']];
  var STATUS_LABEL = { PAID_AWAITING_HOTEL: 'Waiting for reply', CONFIRMED: 'Confirmed', CHECKED_IN: 'Checked in', COMPLETED: 'Completed', EXPIRED: 'Expired', DECLINED_REFUNDED: 'Declined', CANCELLED_REFUNDED: 'Cancelled', NO_SHOW: 'No-show', NEEDS_RELOCATION: 'Needs relocation', DISPUTED: 'Disputed' };
  function statusChip(s) {
    var kind = s === 'PAID_AWAITING_HOTEL' || s === 'NEEDS_RELOCATION' || s === 'DISPUTED' ? 'draft' : s === 'CONFIRMED' || s === 'CHECKED_IN' || s === 'COMPLETED' ? 'active' : 'hidden';
    return h('span', { class: 'chip ' + kind }, STATUS_LABEL[s] || s);
  }
  // actions on one booking; `reload` refreshes the list after
  function bookingActions(b, reload) {
    var wrap = h('div', { class: 'actions' });
    var note = h('span', { class: 'muted', style: 'font-size:13px' });
    function run(promise) { note.textContent = ''; promise.then(reload, function (err) { note.textContent = err.message; }); }
    var today = todayISO();
    if (b.status === 'PAID_AWAITING_HOTEL') {
      var reason = select(DECLINE, 'fully_booked');
      add(wrap, h('button', { onclick: function () { run(call('POST', '/partner/bookings/' + b.id + '/accept')); } }, 'Accept'));
      add(wrap, reason);
      add(wrap, h('button', { class: 'danger', onclick: function () { if (confirm('Decline this booking? The guest is refunded in full.')) run(call('POST', '/partner/bookings/' + b.id + '/decline', { reason: reason.value })); } }, 'Decline'));
    } else if (b.status === 'CONFIRMED') {
      if (b.check_in <= today) {
        add(wrap, h('button', { onclick: function () { run(call('POST', '/partner/bookings/' + b.id + '/check-in')); } }, 'Check in'));
        add(wrap, h('button', { class: 'danger', onclick: function () { if (confirm('Mark this guest as a no-show?')) run(call('POST', '/partner/bookings/' + b.id + '/no-show')); } }, 'No-show'));
      }
      if (state.role === 'owner') {
        var why = select(CANCEL, 'overbooked');
        add(wrap, why);
        add(wrap, h('button', { class: 'danger', onclick: function () { if (confirm('Cancel this booking? The guest is refunded in full and this hotel pays for it.')) run(call('POST', '/partner/bookings/' + b.id + '/cancel', { reason: why.value })); } }, 'Cancel booking'));
      }
    }
    add(wrap, note);
    return wrap;
  }
  function bookingTable(list, reload) {
    var multi = state.hotels.length > 1;
    var table = h('table', {}, h('thead', {}, h('tr', {}, ['Guest', multi ? 'Hotel' : null, 'Room', 'Stay', 'Total', 'Status', ''].filter(Boolean).map(function (t) { return h('th', {}, t); }))));
    var body = h('tbody', {});
    list.forEach(function (b) {
      var g = b.guest || {};
      var detail = [];
      if (b.guest_phone || g.contact) detail.push(b.guest_phone || g.contact);
      if (b.guests) detail.push(b.guests + ' guest' + (b.guests === 1 ? '' : 's'));
      if (b.meal && b.meal !== 'none' && b.meal !== 'room_only') detail.push('breakfast');
      if (b.balance_due_santim) detail.push('collect ' + etb(b.balance_due_santim) + ' at the hotel');
      if (b.special_requests) detail.push('"' + b.special_requests + '"');
      add(body, h('tr', {},
        h('td', {}, h('strong', {}, b.guest_name || g.full_name || 'Guest'), h('div', { class: 'muted', style: 'font-size:12px' }, b.reference), h('div', { class: 'muted', style: 'font-size:12px' }, detail.join(' · '))),
        multi ? h('td', {}, b.property_name || '') : null,
        h('td', {}, b.room_name + (b.rooms > 1 ? ' × ' + b.rooms : '')),
        h('td', {}, day(b.check_in) + ' → ' + day(b.check_out), h('div', { class: 'muted', style: 'font-size:12px' }, b.nights + ' night' + (b.nights === 1 ? '' : 's'))),
        h('td', {}, etb(b.total_santim)),
        h('td', {}, statusChip(b.status), b.decline_reason ? h('div', { class: 'muted', style: 'font-size:12px' }, b.decline_reason) : null),
        h('td', {}, bookingActions(b, reload))));
    });
    add(table, body);
    return h('div', { style: 'overflow-x:auto' }, table);
  }
  function bookingsView() {
    var holder = h('div', {}, loading());
    var filter = 'now';
    var items = [];
    var FILTERS = [['now', 'Needs attention'], ['upcoming', 'Upcoming'], ['house', 'Staying now'], ['past', 'Past'], ['all', 'All']];
    function pick(b) {
      var today = todayISO();
      if (filter === 'now') return b.status === 'PAID_AWAITING_HOTEL' || b.status === 'NEEDS_RELOCATION' || b.status === 'DISPUTED' || (b.status === 'CONFIRMED' && b.check_in <= today);
      if (filter === 'upcoming') return b.status === 'CONFIRMED' || b.status === 'PAID_AWAITING_HOTEL';
      if (filter === 'house') return b.status === 'CHECKED_IN';
      if (filter === 'past') return ['COMPLETED', 'EXPIRED', 'DECLINED_REFUNDED', 'CANCELLED_REFUNDED', 'NO_SHOW'].indexOf(b.status) >= 0;
      return true;
    }
    function paint() {
      clear(holder);
      add(holder, h('div', { class: 'tabs' }, FILTERS.map(function (f) { return h('button', { class: filter === f[0] ? 'on' : '', onclick: function () { filter = f[0]; paint(); } }, f[1]); })));
      var list = items.filter(pick);
      add(holder, h('div', { class: 'card' }, list.length ? bookingTable(list, load) : h('p', { class: 'muted' }, 'Nothing here.')));
    }
    function load() {
      call('GET', '/partner/bookings?limit=200').then(function (res) { items = res.items; paint(); }, function (err) { fail(holder, err); });
    }
    load();
    return holder;
  }

  // ---- availability chart + walk-in / maintenance blocks
  function calendarView() {
    var holder = h('div', {}, loading());
    var from = todayISO();
    function load() {
      var q = '/partner/rooms?days=14&from=' + from + (state.hotelId ? '&property_id=' + encodeURIComponent(state.hotelId) : '');
      Promise.all([call('GET', q), call('GET', '/partner/rooms/blocks' + (state.hotelId ? '?property_id=' + encodeURIComponent(state.hotelId) : ''))]).then(function (res) { paint(res[0], res[1].blocks); }, function (err) { fail(holder, err); });
    }
    function paint(chart, blocks) {
      clear(holder);
      add(holder, h('div', { class: 'row' },
        h('button', { class: 'secondary', onclick: function () { from = shiftISO(from, -14); load(); } }, '← Earlier'),
        h('button', { class: 'secondary', onclick: function () { from = todayISO(); load(); } }, 'Today'),
        h('button', { class: 'secondary', onclick: function () { from = shiftISO(from, 14); load(); } }, 'Later →')));
      var table = h('table', { class: 'chart' });
      var head = h('tr', {}, h('th', {}, 'Room'));
      var dates = chart.rooms.length ? chart.rooms[0].nights.map(function (n) { return n.date; }) : [];
      dates.forEach(function (d) { add(head, h('th', { class: 'c' }, day(d))); });
      add(table, h('thead', {}, head));
      var body = h('tbody', {});
      chart.rooms.forEach(function (r) {
        var tr = h('tr', {}, h('td', {}, h('strong', {}, r.name), h('div', { class: 'muted', style: 'font-size:12px' }, r.total_rooms + ' rooms')));
        r.nights.forEach(function (n) {
          var full = n.free <= 0;
          add(tr, h('td', { class: 'c cell ' + (full ? 'full' : n.booked + n.held + n.blocked > 0 ? 'part' : 'free'), title: n.booked + ' booked, ' + n.held + ' being paid, ' + n.blocked + ' blocked' }, String(n.free)));
        });
        add(body, tr);
      });
      add(table, body);
      add(holder, h('div', { class: 'card' }, h('p', { class: 'soft' }, 'Numbers are rooms still free each night. Green = all free, amber = some taken, red = sold out. Hover a cell for the detail.'), h('div', { style: 'overflow-x:auto' }, table)));
      add(holder, blockForm(chart, load));
      add(holder, h('div', { class: 'card' }, h('h2', {}, 'Walk-ins and blocked rooms'),
        blocks.length ? blocksTable(blocks, load) : h('p', { class: 'muted' }, 'None. Block rooms here when a guest walks in or a room is out of service, so the app stops selling them.')));
    }
    load();
    return holder;
  }
  function shiftISO(iso, n) { var d = new Date(iso + 'T12:00:00'); d.setDate(d.getDate() + n); return d.toISOString().slice(0, 10); }
  function blockForm(chart, reload) {
    if (!chart.rooms.length) return h('div', {});
    var room = h('select', {}, chart.rooms.map(function (r) { return h('option', { value: r.id }, r.name); }));
    var kind = select([['walk_in', 'Walk-in guest'], ['block', 'Out of service / hold']], 'walk_in');
    var inD = h('input', { type: 'date', value: todayISO() }), outD = h('input', { type: 'date', value: todayISO(1) });
    var n = h('input', { value: '1', inputmode: 'numeric' }), name = h('input', { placeholder: 'Guest name (walk-ins)' }), phone = h('input', { placeholder: 'Guest phone' }), note = h('input', { placeholder: 'Note' });
    var out = h('div', {}), btn = h('button', { type: 'submit' }, 'Block rooms');
    return h('div', { class: 'card' }, h('h2', {}, 'Block rooms'), h('form', { onsubmit: function (e) {
      e.preventDefault(); btn.disabled = true; clear(out);
      call('POST', '/partner/rooms/blocks', { property_id: chart.property.id, room_type_id: room.value, check_in: inD.value, check_out: outD.value, rooms: parseInt(n.value, 10) || 1, kind: kind.value, guest_name: name.value, guest_phone: phone.value, note: note.value })
        .then(reload, function (err) { btn.disabled = false; add(out, msg('err', err.message)); });
    } }, h('div', { class: 'grid' }, field('Room', room), field('Type', kind), field('First night', inD), field('Rooms free again on', outD, 'The morning the rooms are free again.'), field('How many rooms', n), field('Guest name', name), field('Guest phone', phone), field('Note', note)), h('div', { class: 'field' }, btn), out));
  }
  function blocksTable(blocks, reload) {
    var table = h('table', {}, h('thead', {}, h('tr', {}, ['Type', 'Room', 'Dates', 'Rooms', 'Who / note', ''].map(function (t) { return h('th', {}, t); }))));
    var body = h('tbody', {});
    blocks.forEach(function (b) {
      add(body, h('tr', {}, h('td', {}, b.kind === 'walk_in' ? 'Walk-in' : 'Blocked'), h('td', {}, b.room_name), h('td', {}, day(b.check_in) + ' → ' + day(b.check_out)), h('td', {}, String(b.rooms)),
        h('td', {}, [b.guest_name, b.guest_phone, b.note].filter(Boolean).join(' · ')),
        h('td', {}, h('button', { class: 'ghost', style: 'color:var(--bad)', onclick: function () { if (confirm('Release these rooms?')) call('DELETE', '/partner/rooms/blocks/' + b.id).then(reload, function (e) { alert(e.message); }); } }, 'Release'))));
    });
    add(table, body);
    return h('div', { style: 'overflow-x:auto' }, table);
  }

  // ---- the hotel's own management pages: the same editor BookDan staff use, limited to what an owner may change
  function hotelPage(active, title, build) {
    var holder = h('div', {}, loading());
    function load() {
      var cur = currentHotel();
      if (!cur) { clear(holder); add(holder, msg('err', 'No hotel is linked to your account.')); return; }
      call('GET', '/partner/hotels/' + cur.id).then(function (hotel) {
        clear(holder);
        add(holder, h('div', { class: 'row between' }, h('div', { class: 'soft' }, 'Changes you save here go live in the app straight away. BookDan can see a record of them.'),
          h('span', { class: 'chip ' + hotel.status }, hotel.status === 'active' ? 'Published' : hotel.status === 'draft' ? 'Draft' : 'Hidden')));
        add(holder, h('div', { class: 'card' }, build(hotel, load)));
      }, function (err) { fail(holder, err); });
    }
    load();
    return holder;
  }

  // ---- map pin: click the map or drag the pin; the two number boxes stay in step (and can still be typed)
  function locationPicker(latBox, lngBox) {
    var holder = h('div', { class: 'mappick' });
    var mapEl = h('div', { class: 'mappick-map' });
    var coords = h('div', { class: 'grid' }, field('Latitude', latBox), field('Longitude', lngBox));
    if (!window.L) { add(holder, coords); return holder; }
    add(holder, mapEl); add(holder, coords);
    var ADDIS = [9.0108, 38.7613];
    function valid() { var a = parseFloat(latBox.value), b = parseFloat(lngBox.value); return isNaN(a) || isNaN(b) ? null : [a, b]; }
    var start = valid();
    setTimeout(function () {
      var map = L.map(mapEl).setView(start || ADDIS, start ? 16 : 12);
      L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', { maxZoom: 19, attribution: '&copy; OpenStreetMap' }).addTo(map);
      var pin = null;
      function set(ll) { latBox.value = ll.lat.toFixed(6); lngBox.value = ll.lng.toFixed(6); }
      function put(ll, fly) {
        if (!pin) {
          pin = L.marker(ll, { draggable: true }).addTo(map);
          pin.on('dragend', function () { set(pin.getLatLng()); });
        } else pin.setLatLng(ll);
        if (fly) map.setView(ll, Math.max(map.getZoom(), 16));
      }
      if (start) put(start, false);
      map.on('click', function (e) { set(e.latlng); put(e.latlng, false); });
      function typed() { var v = valid(); if (v) put(v, true); }
      latBox.addEventListener('change', typed); lngBox.addEventListener('change', typed);
      map.invalidateSize();
    }, 0);
    return holder;
  }

  // ---- hotel form (shared by create and edit)
  function detailsForm(hotel, onSaved) {
    var isNew = !hotel;
    hotel = hotel || { name: '', type: 'hotel', city: '', area: '', address: '', phone: '', description: '', lat: '', lng: '', amenities: [], house_rules: [], pay_at_hotel: false, verified: false, status: 'draft', check_in_time: '14:00', check_out_time: '12:00', description_am: '', directions: '', directions_am: '' };
    var f = {
      name: h('input', { value: hotel.name, maxlength: '120' }),
      type: select(TYPES, hotel.type),
      city: h('input', { value: hotel.city }),
      area: h('input', { value: hotel.area }),
      address: h('input', { value: hotel.address }),
      phone: h('input', { value: hotel.phone, placeholder: '+251 11 123 4567' }),
      lat: h('input', { value: hotel.lat === '' ? '' : String(hotel.lat), inputmode: 'decimal', placeholder: '9.0108' }),
      lng: h('input', { value: hotel.lng === '' ? '' : String(hotel.lng), inputmode: 'decimal', placeholder: '38.7613' }),
      description: h('textarea', { maxlength: '2000' }, hotel.description),
      rules: h('textarea', { placeholder: 'One rule per line' }, (hotel.house_rules || []).join('\n')),
      pay: h('input', { type: 'checkbox', checked: hotel.pay_at_hotel }),
      verified: h('input', { type: 'checkbox', checked: hotel.verified }),
      status: select(STATUSES, hotel.status),
      descAm: h('textarea', { maxlength: '2000', placeholder: 'ስለ ሆቴሉ በአማርኛ' }, hotel.description_am || ''),
      directions: h('textarea', { maxlength: '600', placeholder: 'For example: Behind the blue church, next to the pharmacy' }, hotel.directions || ''),
      directionsAm: h('textarea', { maxlength: '600', placeholder: 'እንዴት እንደሚገኝ በአማርኛ' }, hotel.directions_am || ''),
      checkIn: h('input', { type: 'time', value: hotel.check_in_time || '14:00' }),
      checkOut: h('input', { type: 'time', value: hotel.check_out_time || '12:00' })
    };
    var amen = AMENITIES.map(function (a) { return { key: a[0], box: h('input', { type: 'checkbox', checked: (hotel.amenities || []).indexOf(a[0]) >= 0 }), label: a[1] }; });
    var out = h('div', {});
    var btn = h('button', { type: 'submit' }, isNew ? 'Create hotel' : 'Save changes');
    var form = h('form', { onsubmit: function (e) {
      e.preventDefault(); btn.disabled = true; clear(out);
      var body = {
        name: f.name.value, type: f.type.value, city: f.city.value, area: f.area.value, address: f.address.value, phone: f.phone.value,
        description: f.description.value, lat: parseFloat(f.lat.value), lng: parseFloat(f.lng.value),
        amenities: amen.filter(function (a) { return a.box.checked; }).map(function (a) { return a.key; }),
        house_rules: f.rules.value.split('\n'), pay_at_hotel: f.pay.checked, status: hotel.status,
        check_in_time: f.checkIn.value, check_out_time: f.checkOut.value,
        description_am: f.descAm.value, directions: f.directions.value, directions_am: f.directionsAm.value
      };
      if (isNaN(body.lat)) body.lat = null;
      if (isNaN(body.lng)) body.lng = null;
      var req = isNew ? call('POST', '/admin/hotels', body) : call('PUT', '/partner/hotels/' + hotel.id, body).catch(function (err) {
        // Taking a hotel off the list while guests have bookings to come: show who is affected, and go ahead only on a clear yes.
        if (err.code !== 'HAS_UPCOMING_BOOKINGS' || !confirm(err.message + '\n\nTake the hotel off the list anyway? Those bookings stay valid.')) throw err;
        body.confirm_hide = true;
        return call('PUT', '/partner/hotels/' + hotel.id, body);
      });
      req.then(function (res) { btn.disabled = false; if (isNew) go('#/hotel/' + res.id); else { add(out, msg('ok', 'Saved.')); onSaved && onSaved(); } },
        function (err) { btn.disabled = false; add(out, msg('err', err.message)); });
    } },
      h('div', { class: 'grid' },
        field('Name', f.name), field('Type', f.type), field('City', f.city), field('Area or neighbourhood', f.area),
        field('Street address', f.address), field('Phone for guests', f.phone),
        field('Check-in from', f.checkIn, 'Shown to guests on the hotel page and in their booking (Addis Ababa time).'), field('Check-out until', f.checkOut)),
      field('Location on the map', locationPicker(f.lat, f.lng), 'Click the map (or drag the pin) where the hotel is. Guests see this pin on the hotel page and in the map view.'),
      field('Description', f.description),
      field('Description in Amharic (shown to guests who use the app in Amharic)', f.descAm),
      field('How to find us, in words', f.directions, 'Addresses here are often landmarks. Example: "Behind the blue church, next to the pharmacy". Shown on the hotel page.'),
      field('How to find us, in Amharic', f.directionsAm),
      field('House rules', f.rules),
      h('div', { class: 'field' }, h('label', {}, 'Features'), h('div', { class: 'checks' }, amen.map(function (a) { return h('label', { class: 'check' }, a.box, a.label); }))),
      h('div', { class: 'field' }, h('label', { class: 'check' }, f.pay, 'Guests may choose to pay at the hotel')),
      h('div', { class: 'field' }, h('label', {}, 'Status'), h('div', {}, h('span', { class: 'chip ' + hotel.status }, hotel.status === 'active' ? 'Published' : hotel.status === 'draft' ? 'Draft' : 'Hidden')), h('div', { class: 'hint' }, 'BookDan publishes and hides hotels and checks the business details. Contact BookDan from the Help page to change this.')),
      h('div', { class: 'field' }, btn), out);
    return form;
  }
  function select(options, value) {
    var el = h('select', {}, options.map(function (o) { return h('option', { value: o[0] }, o[1]); }));
    el.value = value; return el;
  }
  function field(label, input, hint) { return h('div', { class: 'field' }, h('label', {}, label), input, hint ? h('div', { class: 'hint' }, hint) : null); }

  // Empty means "not offered"; otherwise the amount in birr as santim, or a percent as basis points.
  function optionalBirr(v) { var n = parseFloat(String(v).replace(/,/g, '')); return isNaN(n) || n <= 0 ? null : Math.round(n * 100); }
  function optionalPercent(v) { var n = parseFloat(v); return isNaN(n) || n <= 0 ? null : Math.round(n * 100); }

  function roomsPanel(hotel, reload) {
    var wrap = h('div', {});
    add(wrap, h('p', { class: 'soft' }, 'Set how many rooms of each kind you have and the price per night in birr. The price applies from today for the next two years. Bookings already made keep the price they were sold at.'));
    add(wrap, h('p', { class: 'soft' }, 'Optional extras guests can choose: a price for breakfast (added per room per night) and a discount for guests who accept no refund. Leave them empty to offer neither.'));
    var out = h('div', {});
    if (hotel.rooms.length) {
      var table = h('table', {}, h('thead', {}, h('tr', {}, ['Room name', 'Guests per room', 'Rooms', 'Features (comma separated)', 'Price per night (ETB)', 'Breakfast per night (ETB)', 'Non-refundable discount (%)', ''].map(function (t) { return h('th', {}, t); }))));
      var body = h('tbody', {});
      hotel.rooms.forEach(function (r) {
        var name = h('input', { value: r.name }), cap = h('input', { value: r.capacity, inputmode: 'numeric', style: 'width:80px' }),
          total = h('input', { value: r.total_rooms, inputmode: 'numeric', style: 'width:80px' }),
          feat = h('input', { value: r.amenities.join(', ') }), price = h('input', { value: birr(r.price_santim), inputmode: 'numeric', style: 'width:120px', placeholder: 'e.g. 3200' }),
          bf = h('input', { value: r.breakfast_santim ? birr(r.breakfast_santim) : '', inputmode: 'numeric', style: 'width:110px', placeholder: 'not offered' }),
          nr = h('input', { value: r.nonrefundable_bps ? String(r.nonrefundable_bps / 100) : '', inputmode: 'numeric', style: 'width:110px', placeholder: 'not offered' });
        var photosRow = h('tr', { style: 'display:none' }, h('td', { colspan: '8' }, roomPhotosPanel(hotel, r, reload)));
        var pbtn = h('button', { class: 'ghost', onclick: function () { photosRow.style.display = photosRow.style.display === 'none' ? '' : 'none'; } }, 'Photos (' + (r.photos || []).length + ')');
        var btn = h('button', { class: 'secondary', onclick: function () {
          btn.disabled = true; clear(out);
          var info = { name: name.value, capacity: parseInt(cap.value, 10), total_rooms: parseInt(total.value, 10), amenities: feat.value.split(','), breakfast_santim: optionalBirr(bf.value), nonrefundable_bps: optionalPercent(nr.value) };
          call('PUT', '/partner/hotels/' + hotel.id + '/rooms/' + r.id, info).then(function () {
            var p = parseInt(price.value, 10);
            if (!isNaN(p) && p * 100 !== r.price_santim) return call('PUT', '/partner/hotels/' + hotel.id + '/rooms/' + r.id + '/price', { price_santim: p * 100 });
          }).then(function () { reload(); }, function (err) { btn.disabled = false; add(out, msg('err', r.name + ': ' + err.message)); });
        } }, 'Save');
        add(body, h('tr', {}, h('td', {}, name), h('td', {}, cap), h('td', {}, total), h('td', {}, feat), h('td', {}, price), h('td', {}, bf), h('td', {}, nr), h('td', {}, btn, ' ', pbtn)));
        add(body, photosRow);
      });
      add(table, body);
      add(wrap, h('div', { style: 'overflow-x:auto' }, table));
    } else add(wrap, h('p', {}, 'No rooms yet. Add the first one below.'));
    add(wrap, out);

    var n = h('input', { placeholder: 'e.g. Deluxe Double' }), c = h('input', { value: '2', inputmode: 'numeric' }), t = h('input', { value: '5', inputmode: 'numeric' }),
      f = h('input', { placeholder: 'Private bathroom, TV' }), p = h('input', { inputmode: 'numeric', placeholder: 'e.g. 3200' }), nb = h('input', { inputmode: 'numeric', placeholder: 'optional' }), nn = h('input', { inputmode: 'numeric', placeholder: 'optional, e.g. 10' }), o2 = h('div', {});
    var add_ = h('button', { type: 'submit' }, 'Add room');
    add(wrap, h('h3', { style: 'margin-top:24px' }, 'Add a room type'));
    add(wrap, h('form', { onsubmit: function (e) {
      e.preventDefault(); add_.disabled = true; clear(o2);
      var price = parseInt(p.value, 10);
      call('POST', '/partner/hotels/' + hotel.id + '/rooms', { name: n.value, capacity: parseInt(c.value, 10), total_rooms: parseInt(t.value, 10), amenities: f.value.split(','), breakfast_santim: optionalBirr(nb.value), nonrefundable_bps: optionalPercent(nn.value) }).then(function (res) {
        if (!isNaN(price)) return call('PUT', '/partner/hotels/' + hotel.id + '/rooms/' + res.id + '/price', { price_santim: price * 100 });
      }).then(function () { reload(); }, function (err) { add_.disabled = false; add(o2, msg('err', err.message)); });
    } },
      h('div', { class: 'grid' }, field('Room name', n), field('Guests per room', c), field('How many rooms', t), field('Features', f), field('Price per night (ETB)', p), field('Breakfast per night (ETB)', nb), field('Non-refundable discount (%)', nn)),
      h('div', { class: 'field' }, add_), o2));
    return wrap;
  }

  // One room's own pictures: they show on that room's card in the app (the hotel's gallery is the Photos tab).
  function roomPhotosPanel(hotel, room, reload) {
    var wrap = h('div', { style: 'padding:8px 0' });
    var out = h('div', {});
    var input = h('input', { type: 'file', accept: 'image/jpeg,image/png,image/webp', multiple: true });
    var btn = h('button', { class: 'secondary' }, 'Upload photos of ' + room.name);
    btn.addEventListener('click', function () {
      var files = Array.prototype.slice.call(input.files || []);
      if (!files.length) { clear(out); add(out, msg('err', 'Choose one or more photos first.')); return; }
      btn.disabled = true; clear(out);
      var i = 0;
      (function next() {
        if (i >= files.length) { reload(); return; }
        var fd = new FormData(); fd.append('photo', files[i]);
        add(out, h('div', { class: 'muted' }, 'Uploading ' + (i + 1) + ' of ' + files.length + '…'));
        call('POST', '/partner/hotels/' + hotel.id + '/rooms/' + room.id + '/photos', fd).then(function () { i++; next(); },
          function (err) { btn.disabled = false; clear(out); add(out, msg('err', files[i].name + ': ' + err.message)); });
      })();
    });
    add(wrap, h('div', { class: 'row' }, input, btn));
    add(wrap, out);
    var grid = h('div', { class: 'photos' });
    (room.photos || []).forEach(function (ph) {
      add(grid, h('div', { class: 'photo' }, h('img', { src: ph.thumb, alt: room.name, loading: 'lazy' }),
        h('div', { class: 'bar' },
          ph.cover ? h('span', { class: 'cover' }, '★ First') : h('button', { class: 'ghost', onclick: function () { call('POST', '/partner/hotels/' + hotel.id + '/photos/' + ph.id + '/cover').then(reload, function (e) { alert(e.message); }); } }, 'Make first'),
          h('button', { class: 'ghost', style: 'color:var(--bad)', onclick: function () { if (confirm('Delete this photo?')) call('DELETE', '/partner/hotels/' + hotel.id + '/photos/' + ph.id).then(reload, function (e) { alert(e.message); }); } }, 'Delete'))));
    });
    add(wrap, (room.photos || []).length ? grid : h('p', { class: 'muted' }, 'No photos of this room yet. Up to 10.'));
    return wrap;
  }

  function photosPanel(hotel, reload) {
    var wrap = h('div', {});
    var out = h('div', {});
    add(wrap, h('p', { class: 'soft' }, 'The first photo is the cover shown in search results. Use JPEG, PNG or WebP, up to 10 MB each (up to 15 photos). They are shrunk automatically so the app stays fast on mobile data.'));
    var input = h('input', { type: 'file', accept: 'image/jpeg,image/png,image/webp', multiple: true });
    var btn = h('button', {}, 'Upload photos');
    btn.addEventListener('click', function () {
      var files = Array.prototype.slice.call(input.files || []);
      if (!files.length) { clear(out); add(out, msg('err', 'Choose one or more photos first.')); return; }
      btn.disabled = true; clear(out);
      var i = 0;
      (function next() {
        if (i >= files.length) { reload(); return; }
        var fd = new FormData(); fd.append('photo', files[i]);
        add(out, h('div', { class: 'muted' }, 'Uploading ' + (i + 1) + ' of ' + files.length + '…'));
        call('POST', '/partner/hotels/' + hotel.id + '/photos', fd).then(function () { i++; next(); },
          function (err) { btn.disabled = false; clear(out); add(out, msg('err', files[i].name + ': ' + err.message)); });
      })();
    });
    add(wrap, h('div', { class: 'row' }, input, btn));
    add(wrap, out);
    var grid = h('div', { class: 'photos' });
    hotel.photos.forEach(function (p) {
      add(grid, h('div', { class: 'photo' }, h('img', { src: p.thumb, alt: 'Hotel photo', loading: 'lazy' }),
        h('div', { class: 'bar' },
          p.cover ? h('span', { class: 'cover' }, '★ Cover') : h('button', { class: 'ghost', onclick: function () { call('POST', '/partner/hotels/' + hotel.id + '/photos/' + p.id + '/cover').then(reload, function (e) { alert(e.message); }); } }, 'Make cover'),
          h('button', { class: 'ghost', style: 'color:var(--bad)', onclick: function () { if (confirm('Delete this photo?')) call('DELETE', '/partner/hotels/' + hotel.id + '/photos/' + p.id).then(reload, function (e) { alert(e.message); }); } }, 'Delete'))));
    });
    add(wrap, hotel.photos.length ? grid : h('p', { class: 'muted' }, 'No photos yet.'));
    return wrap;
  }

  function when(iso) { return new Date(iso).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' }); }

  function staffPanel(hotel, reload) {
    var isAdmin = state.role === 'admin';
    var wrap = h('div', {});
    add(wrap, h('p', { class: 'soft' }, isAdmin
      ? 'Owners manage this hotel here and answer booking requests in the hotel app. Receptionists only answer requests in the hotel app. Everyone gets an invitation email and signs in to the hotel app with the email you enter here.'
      : 'Add your receptionists here: they answer booking requests in the hotel app. They get an invitation email and sign in with that email. Owners are added by BookDan.'));
    if (hotel.staff.length) {
      var table = h('table', {}, h('thead', {}, h('tr', {}, h('th', {}, 'Person'), h('th', {}, 'Role'), h('th', {}, 'Status'), h('th', {}))));
      var body = h('tbody', {});
      hotel.staff.forEach(function (p) {
        var canRemove = isAdmin || p.role !== 'owner';
        add(body, h('tr', {}, h('td', {}, p.contact), h('td', {}, p.role === 'owner' ? 'Owner' : 'Receptionist'),
          h('td', {}, p.status === 'joined' ? h('span', { class: 'chip active' }, 'Joined ' + when(p.joined_at)) : h('span', { class: 'chip draft' }, 'Invited ' + when(p.invited_at))),
          h('td', {}, canRemove ? h('button', { class: 'ghost', style: 'color:var(--bad)', onclick: function () { if (confirm('Remove ' + p.contact + ' from this hotel?')) call('DELETE', '/partner/hotels/' + hotel.id + '/staff/' + p.user_id).then(reload, function (e) { alert(e.message); }); } }, 'Remove') : null)));
      });
      add(table, body); add(wrap, h('div', { style: 'overflow-x:auto' }, table));
      add(wrap, h('p', { class: 'muted', style: 'font-size:13px' }, '"Invited" means they have not signed in yet. If they did not receive the email, check their address and spam folder, or remove and add them again.'));
    } else add(wrap, h('p', {}, 'Nobody works for this hotel yet, so booking requests will not reach anyone.'));
    var contact = h('input', { placeholder: 'name@example.com or +2519…' });
    var role = isAdmin ? select([['owner', 'Owner'], ['receptionist', 'Receptionist']], 'owner') : h('input', { value: 'Receptionist', disabled: true });
    var out = h('div', {});
    var btn = h('button', { type: 'submit' }, 'Add and invite');
    add(wrap, h('h3', { style: 'margin-top:24px' }, 'Add a person'));
    add(wrap, h('form', { onsubmit: function (e) {
      e.preventDefault(); btn.disabled = true; clear(out);
      call('POST', '/partner/hotels/' + hotel.id + '/staff', { contact: contact.value, role: isAdmin ? role.value : 'receptionist' }).then(reload, function (err) { btn.disabled = false; add(out, msg('err', err.message)); });
    } }, h('div', { class: 'grid' }, field('Email or phone', contact, 'An email address gets an invitation. A phone number is added without an email.'), field('Role', role)), h('div', { class: 'field' }, btn), out));
    return wrap;
  }

  // Booking alerts: who is told when a request arrives, and when. Owners and platform admins both manage this.
  function alertsPanel(hotel, reload) {
    var wrap = h('div', {});
    add(wrap, h('p', { class: 'soft' }, 'When a guest books, the people on duty are alerted on their phones at the same time and the first to answer wins. Five minutes before the request runs out, everyone gets a reminder and the owners get an email.'));
    var mode = select([['everyone', 'Everyone on duty at once'], ['front_desk_first', 'Receptionists first, owners only if nobody answers']], hotel.alert_mode);
    var wait = h('input', { type: 'number', min: '1', max: '10', value: String(hotel.escalate_after_min) });
    var out = h('div', {});
    var save = h('button', { type: 'submit' }, 'Save rules');
    add(wrap, h('form', { onsubmit: function (e) {
      e.preventDefault(); save.disabled = true; clear(out);
      call('PUT', '/partner/hotels/' + hotel.id + '/alerts', { alert_mode: mode.value, escalate_after_min: parseInt(wait.value, 10) || 0 }).then(reload, function (err) { save.disabled = false; add(out, msg('err', err.message)); });
    } }, h('div', { class: 'grid' }, field('Who is alerted first', mode, 'If nobody in that group has the hotel app signed in or is on duty, the next group is alerted straight away.'),
      field('Alert the owners after (minutes)', wait, 'Only for "receptionists first". The request lasts 15 minutes in total.')), h('div', { class: 'field' }, save), out));

    add(wrap, h('h3', { style: 'margin-top:24px' }, 'People and shifts'));
    add(wrap, h('p', { class: 'muted', style: 'font-size:13px' }, 'Shifts use Addis Ababa time. Leave both times empty for always on duty. A shift can pass midnight (22:00 to 06:00). Owners are still alerted outside their shift when nobody else answers; switch their alerts off to stop that.'));
    if (!hotel.staff.length) { add(wrap, h('p', {}, 'Nobody works for this hotel yet, so booking requests will not reach anyone.')); return wrap; }
    var table = h('table', {}, h('thead', {}, h('tr', {}, h('th', {}, 'Person'), h('th', {}, 'Phone'), h('th', {}, 'Alerts'), h('th', {}, 'From'), h('th', {}, 'Until'), h('th', {}))));
    var body = h('tbody', {});
    hotel.staff.forEach(function (p) {
      var on = h('input', { type: 'checkbox' }); on.checked = p.alerts_on;
      var from = h('input', { type: 'time', value: p.shift_start || '' }), to = h('input', { type: 'time', value: p.shift_end || '' });
      var note = h('span', { class: 'muted', style: 'font-size:13px' });
      var canEdit = state.role === 'admin' || p.role !== 'owner' || p.contact === state.contact;
      var btn = h('button', { class: 'secondary', disabled: !canEdit, onclick: function () {
        note.textContent = '';
        call('PUT', '/partner/hotels/' + hotel.id + '/staff/' + p.user_id + '/alerts', { alerts_on: on.checked, shift_start: from.value, shift_end: to.value })
          .then(function () { note.textContent = 'Saved'; }, function (e) { note.textContent = e.message; });
      } }, 'Save');
      add(body, h('tr', {}, h('td', {}, p.contact + (p.role === 'owner' ? ' (owner)' : '')),
        h('td', {}, p.reachable ? h('span', { class: 'chip active' }, 'Signed in') : h('span', { class: 'chip draft' }, 'No phone')),
        h('td', {}, on), h('td', {}, from), h('td', {}, to), h('td', {}, btn, ' ', note)));
    });
    add(table, body); add(wrap, h('div', { style: 'overflow-x:auto' }, table));
    add(wrap, h('p', { class: 'muted', style: 'font-size:13px' }, '"No phone" means the person has not signed in to the hotel app on a phone that allows notifications, so they cannot be alerted.'));
    return wrap;
  }



  // ---- earnings (owners)
  function earningsView() {
    var holder = h('div', {}, loading());
    call('GET', '/partner/earnings').then(function (res) {
      clear(holder);
      res.hotels.forEach(function (x) {
        if (state.hotels.length > 1 && state.hotelId && x.id !== state.hotelId) return;
        add(holder, h('div', { class: 'tiles' },
          [['BookDan owes you now', etb(x.to_pay_santim), 'ok'], ['Ready (guest arrived or stay over)', etb(x.ready_santim), ''], ['Stays still to come', etb(x.upcoming_santim), ''], ['Already paid to you', etb(x.paid_santim), '']].map(function (t) { return h('div', { class: 'tile ' + t[2] }, h('div', { class: 'num' }, t[1]), h('div', { class: 'soft' }, t[0])); })));
        if (x.owes_santim) add(holder, msg('warn', 'You owe BookDan ' + etb(x.owes_santim) + ': the fee on stays that guests paid at the hotel is more than BookDan owes you.'));
        var lines = h('table', {}, h('thead', {}, h('tr', {}, ['Guest', 'Stay', 'Guest paid', 'BookDan fee', 'VAT on fee', 'You receive', ''].map(function (t) { return h('th', {}, t); }))));
        var body = h('tbody', {});
        x.items.forEach(function (i) {
          add(body, h('tr', {}, h('td', {}, i.guest_name, h('div', { class: 'muted', style: 'font-size:12px' }, i.reference)), h('td', {}, day(i.check_in) + ' → ' + day(i.check_out)), h('td', {}, etb(i.gross_santim)),
            h('td', {}, etb(i.commission_santim)), h('td', {}, etb(i.vat_santim)), h('td', {}, h('strong', {}, etb(i.net_santim))), h('td', {}, i.ready ? h('span', { class: 'chip active' }, 'Payable') : h('span', { class: 'chip draft' }, 'Not yet'))));
        });
        add(lines, body);
        add(holder, h('div', { class: 'card' }, h('h2', {}, x.name + ': bookings'), x.items.length ? h('div', { style: 'overflow-x:auto' }, lines) : h('p', { class: 'muted' }, 'No paid bookings yet.')));
        var pay = h('table', {}, h('thead', {}, h('tr', {}, ['Paid on', 'Amount', 'Method', 'Reference'].map(function (t) { return h('th', {}, t); }))));
        var pb = h('tbody', {});
        x.payouts.forEach(function (p) { add(pb, h('tr', {}, h('td', {}, day(p.paid_at)), h('td', {}, etb(p.amount_santim)), h('td', {}, p.method), h('td', {}, p.reference))); });
        add(pay, pb);
        add(holder, h('div', { class: 'card' }, h('h2', {}, 'Payments BookDan made to you'), x.payouts.length ? h('div', { style: 'overflow-x:auto' }, pay) : h('p', { class: 'muted' }, 'No payment yet.')));
      });
    }, function (err) { fail(holder, err); });
    return holder;
  }

  function statementsView() {
    var holder = h('div', {}, loading());
    var month = '';
    function load() {
      call('GET', '/partner/statements' + (month ? '?month=' + month : '') + (state.hotelId ? (month ? '&' : '?') + 'property_id=' + encodeURIComponent(state.hotelId) : '')).then(function (res) {
        clear(holder);
        var m = h('input', { type: 'month', value: month || (res.statements[0] && res.statements[0].month) || '' });
        add(holder, h('div', { class: 'row' }, h('label', { style: 'margin:0' }, 'Month'), m, h('button', { class: 'secondary', onclick: function () { month = m.value; load(); } }, 'Show')));
        res.statements.forEach(function (s) {
          add(holder, h('div', { class: 'card' }, h('h2', {}, s.hotel + ' · ' + s.month),
            h('div', { class: 'tiles' }, [['Balance at start', etb(s.opening_santim)], ['Online hotel share', etb(s.online_hotel_share_santim)], ['Paid to you', etb(s.paid_santim)], ['Balance at end', etb(s.closing_santim)]].map(function (t) { return h('div', { class: 'tile' }, h('div', { class: 'num' }, t[1]), h('div', { class: 'soft' }, t[0])); })),
            s.lines.length ? statementTable(s) : h('p', { class: 'muted' }, 'No stays ended in this month.')));
        });
      }, function (err) { fail(holder, err); });
    }
    load();
    return holder;
  }
  function statementTable(s) {
    var t = h('table', {}, h('thead', {}, h('tr', {}, ['Date', 'Guest', 'Nights', 'Paid', 'Guest paid', 'BookDan fee', 'VAT', 'Status'].map(function (x) { return h('th', {}, x); }))));
    var b = h('tbody', {});
    s.lines.forEach(function (l) {
      add(b, h('tr', {}, h('td', {}, day(l.date)), h('td', {}, l.guest_name, h('div', { class: 'muted', style: 'font-size:12px' }, l.reference)), h('td', {}, String(l.nights)), h('td', {}, l.method === 'online' ? 'Online' : 'At hotel'),
        h('td', {}, etb(l.gross_santim)), h('td', {}, etb(l.commission_santim)), h('td', {}, etb(l.vat_santim)), h('td', {}, l.status)));
    });
    add(t, b);
    return h('div', { style: 'overflow-x:auto' }, t);
  }

  // ---- reviews and replies
  function reviewsView() {
    var holder = h('div', {}, loading());
    function load() {
      call('GET', '/partner/reviews').then(function (res) {
        clear(holder);
        if (!res.reviews.length) { add(holder, h('div', { class: 'card' }, h('p', { class: 'muted' }, 'No reviews yet.'))); return; }
        res.reviews.forEach(function (r) {
          var box = h('div', { class: 'card' });
          add(box, h('div', { class: 'row between' }, h('div', {}, h('strong', {}, r.author), ' ', h('span', { class: 'muted' }, r.hotel + ' · ' + day(r.date))), h('span', { class: 'chip active' }, '★ ' + r.rating + '/5')));
          add(box, h('p', {}, r.text));
          if (r.reply) {
            add(box, h('div', { class: 'reply' }, h('div', { class: 'soft', style: 'font-size:13px' }, 'Your reply · ' + day(r.reply.date)), h('p', {}, r.reply.text),
              h('button', { class: 'ghost', style: 'color:var(--bad)', onclick: function () { if (confirm('Delete your reply?')) call('DELETE', '/partner/reviews/' + r.id + '/reply').then(load, function (e) { alert(e.message); }); } }, 'Delete reply')));
          } else {
            var text = h('textarea', { placeholder: 'Reply politely and briefly. Everyone can read it.', maxlength: '1000' }), out = h('div', {}), btn = h('button', {}, 'Post reply');
            btn.addEventListener('click', function () { btn.disabled = true; clear(out); call('PUT', '/partner/reviews/' + r.id + '/reply', { text: text.value }).then(load, function (err) { btn.disabled = false; add(out, msg('err', err.message)); }); });
            add(box, text); add(box, h('div', { class: 'field' }, btn)); add(box, out);
          }
          add(holder, box);
        });
      }, function (err) { fail(holder, err); });
    }
    load();
    return holder;
  }

  // ---- help from BookDan
  function helpView() {
    var holder = h('div', {}, loading());
    function load() {
      call('GET', '/partner/support/tickets').then(function (res) {
        clear(holder);
        var subject = h('input', { maxlength: '120', placeholder: 'What do you need help with?' }), body = h('textarea', { placeholder: 'Tell us what happened', maxlength: '2000' }), kind = select([['question', 'A question'], ['problem', 'A problem']], 'question');
        var out = h('div', {}), btn = h('button', { type: 'submit' }, 'Send to BookDan');
        add(holder, h('div', { class: 'card' }, h('h2', {}, 'Ask BookDan'), h('form', { onsubmit: function (e) {
          e.preventDefault(); btn.disabled = true; clear(out);
          call('POST', '/partner/support/tickets', { kind: kind.value, subject: subject.value, message: body.value }).then(load, function (err) { btn.disabled = false; add(out, msg('err', err.message)); });
        } }, h('div', { class: 'grid' }, field('Kind', kind), field('Subject', subject)), field('Message', body), h('div', { class: 'field' }, btn), out)));
        var list = h('div', { class: 'card' }, h('h2', {}, 'Your conversations'));
        if (!res.tickets.length) add(list, h('p', { class: 'muted' }, 'Nothing yet.'));
        res.tickets.forEach(function (t) {
          var det = h('div', {});
          var row = h('div', { class: 'ticket', tabindex: '0', onclick: function () { if (det.firstChild) { clear(det); return; } openTicket(t.id, det, load); } },
            h('div', { class: 'row between' }, h('strong', {}, t.subject), h('span', { class: 'chip ' + (t.status === 'closed' ? 'hidden' : 'active') }, t.status)), h('div', { class: 'muted', style: 'font-size:13px' }, t.preview || ''));
          add(list, row); add(list, det);
        });
        add(holder, list);
      }, function (err) { fail(holder, err); });
    }
    load();
    return holder;
  }
  function openTicket(id, into, reload) {
    add(into, loading());
    call('GET', '/partner/support/tickets/' + id).then(function (res) {
      clear(into);
      var t = res.ticket;
      (t.messages || []).forEach(function (m) { add(into, h('div', { class: 'bubble ' + (m.role === 'support' ? 'them' : 'me') }, h('div', { class: 'muted', style: 'font-size:12px' }, m.author + ' · ' + day(m.created_at)), h('div', {}, m.body))); });
      if (t.status !== 'closed') {
        var text = h('textarea', { placeholder: 'Write a reply', maxlength: '2000' }), out = h('div', {});
        var send = h('button', {}, 'Send'), close = h('button', { class: 'secondary' }, 'Close conversation');
        send.addEventListener('click', function (e) { e.stopPropagation(); send.disabled = true; call('POST', '/partner/support/tickets/' + id + '/messages', { message: text.value }).then(function () { clear(into); openTicket(id, into, reload); }, function (err) { send.disabled = false; add(out, msg('err', err.message)); }); });
        close.addEventListener('click', function (e) { e.stopPropagation(); call('POST', '/partner/support/tickets/' + id + '/close').then(reload, function (err) { add(out, msg('err', err.message)); }); });
        text.addEventListener('click', function (e) { e.stopPropagation(); });
        add(into, text); add(into, h('div', { class: 'row', style: 'margin-top:8px' }, send, close)); add(into, out);
      }
    }, function (err) { clear(into); add(into, msg('err', err.message)); });
  }

  // ---- router
  function render() {
    clear(app);
    if (!state.token) { add(app, loginView()); return; }
    var route = location.hash || '#/';
    var holder = h('div', {}, loading());
    add(app, holder);
    call('GET', '/partner/hotels').then(function (res) {
      state.hotels = res.items || [];
      state.role = state.hotels.length ? 'owner' : 'staff';
      if (!currentHotel() || (state.hotelId && !state.hotels.some(function (x) { return x.id === state.hotelId; }))) { state.hotelId = state.hotels[0] ? state.hotels[0].id : ''; }
      paint(route);
    }, function (err) {
      // A signed-in person who is not hotel staff at all is told so, not shown an empty portal.
      clear(app);
      if (err.status === 401 || !state.token) { add(app, loginView()); return; }
      add(app, h('div', { class: 'card login' }, h('h1', {}, 'Hotel Portal'), msg('err', err.message), h('p', { class: 'soft' }, 'Your account is not linked to a hotel yet. Ask BookDan to add you, then sign in again.'), h('button', { onclick: signOut }, 'Sign out')));
    });
  }
  function paint(route) {
    var views = {
      '#/': ['Home', homeView], '#/bookings': ['Bookings', bookingsView], '#/calendar': ['Availability', calendarView], '#/help': ['Help from BookDan', helpView],
      '#/hotel': ['My hotel', function () { return hotelPage(route, 'My hotel', detailsForm); }],
      '#/rooms': ['Rooms & prices', function () { return hotelPage(route, 'Rooms', roomsPanel); }],
      '#/photos': ['Photos', function () { return hotelPage(route, 'Photos', photosPanel); }],
      '#/team': ['Team', function () { return hotelPage(route, 'Team', staffPanel); }],
      '#/alerts': ['Booking alerts', function () { return hotelPage(route, 'Alerts', alertsPanel); }],
      '#/earnings': ['Earnings', earningsView], '#/statements': ['Statements', statementsView], '#/reviews': ['Reviews', reviewsView]
    };
    var ownerOnly = ['#/hotel', '#/rooms', '#/photos', '#/team', '#/alerts', '#/earnings', '#/statements', '#/reviews'];
    if (!views[route] || (state.role !== 'owner' && ownerOnly.indexOf(route) >= 0)) route = '#/';
    var v = views[route];
    clear(app);
    add(app, shell(route, v[0], v[1]()));
  }

  window.addEventListener('hashchange', render);
  render();
})();
