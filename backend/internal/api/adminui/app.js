// BookDan admin page. Plain JavaScript, no libraries. Every piece of text from the server is put on the page with textContent,
// never as HTML, so a hotel name can never run code.
(function () {
  'use strict';

  var API = '/v1';
  var AMENITIES = [['generator', 'Generator'], ['hot_water', 'Hot water'], ['wifi', 'Wi-Fi'], ['parking', 'Parking'], ['pay_at_hotel', 'Pay at hotel'], ['ac', 'Air conditioning'], ['pool', 'Swimming pool'], ['restaurant', 'Restaurant'], ['gym', 'Gym'], ['spa', 'Spa'], ['airport_shuttle', 'Airport shuttle'], ['laundry', 'Laundry'], ['front_desk_24h', '24-hour front desk'], ['water_tank', '24-hour water']];
  var TYPES = [['hotel', 'Hotel'], ['guesthouse', 'Guesthouse'], ['lodge', 'Lodge'], ['resort', 'Resort']];
  var STATUSES = [['draft', 'Draft (not shown to guests)'], ['active', 'Published'], ['hidden', 'Hidden']];

  var state = { token: null, role: null, contact: '', adminRole: '', can: {}, mfa: false, hasAccount: false, mustChange: false, config: { password: false, owners: false } };
  try { state.token = sessionStorage.getItem('admin-token'); state.role = sessionStorage.getItem('admin-role'); } catch (e) { /* private mode */ }
  var app = document.getElementById('app');

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

  // ---- server calls
  function call(method, path, body) {
    var opts = { method: method, headers: {} };
    if (state.token) opts.headers.Authorization = 'Bearer ' + state.token;
    if (body instanceof FormData) opts.body = body;
    else if (body !== undefined) { opts.headers['Content-Type'] = 'application/json'; opts.body = JSON.stringify(body); }
    return fetch(API + path, opts).then(function (res) {
      return res.json().catch(function () { return {}; }).then(function (json) {
        if (res.ok) return json;
        if (res.status === 401 && state.token) { signOut(); }
        var e = new Error((json.error && json.error.message) || 'Something went wrong (' + res.status + ')');
        e.status = res.status;
        e.code = json.error && json.error.code;
        throw e;
      });
    }, function () { throw new Error('No connection to the server'); });
  }
  function signOut() {
    state.token = state.role = null;
    try { sessionStorage.removeItem('admin-token'); sessionStorage.removeItem('admin-role'); } catch (e) { /* ignore */ }
    render();
  }
  function signedIn(res) {
    state.token = res.token; state.role = res.role;
    try { sessionStorage.setItem('admin-token', res.token); sessionStorage.setItem('admin-role', res.role); } catch (e) { /* ignore */ }
    loadMe().then(function () { go('#/'); });
  }
  // loadMe reads who is signed in and what their role may do, so the page shows the right tabs.
  function loadMe() {
    return call('GET', '/admin/me').then(function (me) {
      state.contact = me.contact; state.role = me.role; state.adminRole = me.admin_role || ''; state.can = me.can || {};
      state.mfa = !!me.mfa; state.hasAccount = !!me.has_account; state.mustChange = !!me.must_change_password;
    }, function () { /* signed out by call() */ });
  }
  function can(what) { return state.role === 'admin' && !!state.can[what]; }

  // ---- login
  function loginView() {
    var tab = state.config.password ? 'admin' : 'owner';
    var box = h('div', { class: 'card login' });
    function paint(note) {
      clear(box);
      add(box, h('h1', {}, 'BookDan Admin'));
      add(box, h('p', { class: 'soft' }, 'Manage hotels, rooms, prices and photos.'));
      if (state.config.password && state.config.owners) {
        add(box, h('div', { class: 'tabs' },
          h('button', { class: tab === 'admin' ? 'on' : '', onclick: function () { tab = 'admin'; paint(); } }, 'Platform admin'),
          h('button', { class: tab === 'owner' ? 'on' : '', onclick: function () { tab = 'owner'; paint(); } }, 'Hotel owner')));
      }
      add(box, tab === 'admin' ? adminForm() : ownerForm());
      if (note) add(box, msg('err', note));
    }
    function adminForm() {
      var email = h('input', { type: 'email', autocomplete: 'username', required: true });
      var pass = h('input', { type: 'password', autocomplete: 'current-password', required: true });
      var btn = h('button', { type: 'submit' }, 'Sign in');
      var code = h('input', { type: 'text', inputmode: 'numeric', maxlength: '6', placeholder: '6-digit code', autocomplete: 'one-time-code' });
      var codeField = h('div', { class: 'field', style: 'display:none' }, h('label', {}, 'Code from your authenticator app'), code);
      var note = h('div', {});
      return h('form', { onsubmit: function (e) {
        e.preventDefault(); btn.disabled = true; clear(note);
        call('POST', '/admin/login', { email: email.value, password: pass.value, code: code.value }).then(signedIn, function (err) {
          btn.disabled = false;
          if (err.code === 'MFA_REQUIRED') { codeField.style.display = ''; code.focus(); return; }
          add(note, msg('err', err.message));
        });
      } },
        h('div', { class: 'field' }, h('label', {}, 'Email'), email),
        h('div', { class: 'field' }, h('label', {}, 'Password'), pass),
        codeField, note,
        h('div', { class: 'field' }, btn));
    }
    function ownerForm() {
      var contact = h('input', { type: 'text', placeholder: 'name@example.com or +2519…', autocomplete: 'username' });
      var code = h('input', { type: 'text', inputmode: 'numeric', maxlength: '6', placeholder: '6-digit code', autocomplete: 'one-time-code' });
      var stage = 1;
      var form = h('form', { onsubmit: function (e) {
        e.preventDefault();
        var c = contact.value.trim(), ident = { type: c.indexOf('@') >= 0 ? 'email' : 'phone', value: c };
        if (stage === 1) {
          call('POST', '/admin/otp/request', ident).then(function () { stage = 2; codeField.style.display = ''; btn.textContent = 'Sign in'; }, function (err) { paint(err.message); });
        } else {
          call('POST', '/admin/otp/verify', { type: ident.type, value: ident.value, code: code.value }).then(signedIn, function (err) { paint(err.message); });
        }
      } });
      var codeField = h('div', { class: 'field', style: 'display:none' }, h('label', {}, 'Code we sent you'), code);
      var btn = h('button', { type: 'submit' }, 'Send me a code');
      add(form, [h('div', { class: 'field' }, h('label', {}, 'Your email or phone'), contact), codeField, h('div', { class: 'field' }, btn)]);
      return form;
    }
    paint();
    return h('div', {}, box);
  }

  // ---- shell
  function shell(content) {
    return h('div', {},
      h('header', { class: 'top' },
        h('span', { class: 'brand' }, 'BookDan Admin'),
        h('span', { class: 'who' }, state.contact, ' ', h('span', { class: 'chip' }, state.role === 'admin' ? { super: 'Super admin', support: 'Support', finance: 'Finance' }[state.adminRole] || 'Platform admin' : 'Hotel owner'),
          state.role === 'admin' && !state.mustChange ? h('button', { class: 'ghost', onclick: function () { go('#/account'); } }, 'My account') : null,
          h('button', { class: 'ghost', onclick: signOut }, 'Sign out'))),
      h('main', {}, content));
  }

  // The tabs a platform admin sees: only what their role may open (the server checks it again on every request).
  function navButtons() {
    var tabs = [['Coupons', '#/coupons', null], ['Payouts', '#/payouts', 'finance'], ['Payment issues', '#/issues', 'finance'], ['Support', '#/support', 'support'], ['Reviews', '#/reviews', 'reviews'],
      ['Rates', '#/rates', null], ['Calls', '#/calls', null], ['Analytics', '#/analytics', null], ['Applications', '#/applications', 'applications'], ['Hotel changes', '#/activity', 'admins'], ['Admins', '#/admins', 'admins']];
    var span = h('span', {});
    tabs.forEach(function (t) {
      if (t[2] && !can(t[2])) return;
      add(span, h('button', { class: 'secondary', onclick: function () { go(t[1]); } }, t[0]), ' ');
    });
    if (can('hotels_write')) add(span, h('button', { onclick: function () { go('#/new'); } }, 'Add a hotel'));
    return span;
  }

  // ---- hotel list
  function listView() {
    var holder = h('div', {}, h('p', { class: 'muted' }, 'Loading hotels…'));
    call('GET', '/admin/hotels').then(function (res) {
      clear(holder);
      if (!res.items.length) add(holder, h('div', { class: 'card' }, h('p', {}, state.role === 'admin' ? 'No hotels yet. Add the first one.' : 'You do not own any hotel yet.')));
      res.items.forEach(function (x) {
        add(holder, h('div', { class: 'card hotel', tabindex: '0', onclick: function () { go('#/hotel/' + x.id); },
          onkeydown: function (e) { if (e.key === 'Enter') go('#/hotel/' + x.id); } },
          h('div', { class: 'row between' },
            h('div', {}, h('h3', {}, x.name), h('div', { class: 'soft' }, x.city + ' · ' + x.type + ' · ' + x.room_types + ' room type' + (x.room_types === 1 ? '' : 's') + ' · ' + x.photos + ' photo' + (x.photos === 1 ? '' : 's'))),
            h('span', { class: 'chip ' + x.status }, x.status === 'active' ? 'Published' : x.status === 'draft' ? 'Draft' : 'Hidden'))));
      });
    }, function (err) { clear(holder); add(holder, msg('err', err.message)); });
    return shell(h('div', {},
      h('div', { class: 'row between' }, h('h1', {}, 'Hotels'),
        state.role === 'admin' ? navButtons() : null),
      holder));
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
        house_rules: f.rules.value.split('\n'), pay_at_hotel: f.pay.checked, status: isNew ? 'draft' : f.status.value,
        check_in_time: f.checkIn.value, check_out_time: f.checkOut.value,
        description_am: f.descAm.value, directions: f.directions.value, directions_am: f.directionsAm.value
      };
      if (isNaN(body.lat)) body.lat = null;
      if (isNaN(body.lng)) body.lng = null;
      if (state.role === 'admin') body.verified = f.verified.checked;
      var req = isNew ? call('POST', '/admin/hotels', body) : call('PUT', '/admin/hotels/' + hotel.id, body).catch(function (err) {
        // Taking a hotel off the list while guests have bookings to come: show who is affected, and go ahead only on a clear yes.
        if (err.code !== 'HAS_UPCOMING_BOOKINGS' || !confirm(err.message + '\n\nTake the hotel off the list anyway? Those bookings stay valid.')) throw err;
        body.confirm_hide = true;
        return call('PUT', '/admin/hotels/' + hotel.id, body);
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
      state.role === 'admin' ? h('div', { class: 'field' }, h('label', { class: 'check' }, f.verified, 'Verified by BookDan (shows a "Verified" badge)')) : null,
      isNew ? null : field('Status', f.status, 'To publish a hotel it needs at least one room with a price, and (set by a platform admin) its business and payout details.'),
      h('div', { class: 'field' }, btn), out);
    return form;
  }
  function select(options, value) {
    var el = h('select', {}, options.map(function (o) { return h('option', { value: o[0] }, o[1]); }));
    el.value = value; return el;
  }
  function field(label, input, hint) { return h('div', { class: 'field' }, h('label', {}, label), input, hint ? h('div', { class: 'hint' }, hint) : null); }

  function newView() {
    return shell(h('div', {}, h('button', { class: 'ghost', onclick: function () { go('#/'); } }, '← All hotels'), h('h1', {}, 'Add a hotel'),
      h('p', { class: 'soft' }, 'It starts as a draft. Add rooms, prices and photos, then publish it.'), h('div', { class: 'card' }, detailsForm(null))));
  }

  // ---- hotel editor
  function hotelView(id, tab) {
    var holder = h('div', {}, h('p', { class: 'muted' }, 'Loading…'));
    function load() {
      call('GET', '/admin/hotels/' + id).then(function (hotel) { paint(hotel); }, function (err) { clear(holder); add(holder, msg('err', err.message)); });
    }
    function paint(hotel) {
      clear(holder);
      var tabs = [['details', 'Details'], ['rooms', 'Rooms & prices'], ['photos', 'Photos (' + hotel.photos.length + ')']];
      tabs.push(['staff', 'Staff (' + hotel.staff.length + ')']);
      tabs.push(['alerts', 'Booking alerts']);
      if (state.role === 'admin') tabs.push(['business', hotel.business ? 'Business' : hotel.status === 'active' ? 'Business (not set)' : 'Business (needed to publish)']);
      add(holder, [
        h('div', { class: 'row between' }, h('h1', {}, hotel.name), h('span', { class: 'chip ' + hotel.status }, hotel.status === 'active' ? 'Published' : hotel.status === 'draft' ? 'Draft' : 'Hidden')),
        h('div', { class: 'tabs' }, tabs.map(function (t) { return h('button', { class: tab === t[0] ? 'on' : '', onclick: function () { tab = t[0]; paint(hotel); } }, t[1]); })),
        h('div', { class: 'card' }, tab === 'details' ? detailsForm(hotel, load) : tab === 'rooms' ? roomsPanel(hotel, load) : tab === 'photos' ? photosPanel(hotel, load) : tab === 'business' ? businessPanel(hotel, load) : tab === 'alerts' ? alertsPanel(hotel, load) : staffPanel(hotel, load))
      ]);
    }
    load();
    return shell(h('div', {}, h('button', { class: 'ghost', onclick: function () { go('#/'); } }, '← All hotels'), holder));
  }

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
          call('PUT', '/admin/hotels/' + hotel.id + '/rooms/' + r.id, info).then(function () {
            var p = parseInt(price.value, 10);
            if (!isNaN(p) && p * 100 !== r.price_santim) return call('PUT', '/admin/hotels/' + hotel.id + '/rooms/' + r.id + '/price', { price_santim: p * 100 });
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
      call('POST', '/admin/hotels/' + hotel.id + '/rooms', { name: n.value, capacity: parseInt(c.value, 10), total_rooms: parseInt(t.value, 10), amenities: f.value.split(','), breakfast_santim: optionalBirr(nb.value), nonrefundable_bps: optionalPercent(nn.value) }).then(function (res) {
        if (!isNaN(price)) return call('PUT', '/admin/hotels/' + hotel.id + '/rooms/' + res.id + '/price', { price_santim: price * 100 });
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
        call('POST', '/admin/hotels/' + hotel.id + '/rooms/' + room.id + '/photos', fd).then(function () { i++; next(); },
          function (err) { btn.disabled = false; clear(out); add(out, msg('err', files[i].name + ': ' + err.message)); });
      })();
    });
    add(wrap, h('div', { class: 'row' }, input, btn));
    add(wrap, out);
    var grid = h('div', { class: 'photos' });
    (room.photos || []).forEach(function (ph) {
      add(grid, h('div', { class: 'photo' }, h('img', { src: ph.thumb, alt: room.name, loading: 'lazy' }),
        h('div', { class: 'bar' },
          ph.cover ? h('span', { class: 'cover' }, '★ First') : h('button', { class: 'ghost', onclick: function () { call('POST', '/admin/hotels/' + hotel.id + '/photos/' + ph.id + '/cover').then(reload, function (e) { alert(e.message); }); } }, 'Make first'),
          h('button', { class: 'ghost', style: 'color:var(--bad)', onclick: function () { if (confirm('Delete this photo?')) call('DELETE', '/admin/hotels/' + hotel.id + '/photos/' + ph.id).then(reload, function (e) { alert(e.message); }); } }, 'Delete'))));
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
        call('POST', '/admin/hotels/' + hotel.id + '/photos', fd).then(function () { i++; next(); },
          function (err) { btn.disabled = false; clear(out); add(out, msg('err', files[i].name + ': ' + err.message)); });
      })();
    });
    add(wrap, h('div', { class: 'row' }, input, btn));
    add(wrap, out);
    var grid = h('div', { class: 'photos' });
    hotel.photos.forEach(function (p) {
      add(grid, h('div', { class: 'photo' }, h('img', { src: p.thumb, alt: 'Hotel photo', loading: 'lazy' }),
        h('div', { class: 'bar' },
          p.cover ? h('span', { class: 'cover' }, '★ Cover') : h('button', { class: 'ghost', onclick: function () { call('POST', '/admin/hotels/' + hotel.id + '/photos/' + p.id + '/cover').then(reload, function (e) { alert(e.message); }); } }, 'Make cover'),
          h('button', { class: 'ghost', style: 'color:var(--bad)', onclick: function () { if (confirm('Delete this photo?')) call('DELETE', '/admin/hotels/' + hotel.id + '/photos/' + p.id).then(reload, function (e) { alert(e.message); }); } }, 'Delete'))));
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
          h('td', {}, canRemove ? h('button', { class: 'ghost', style: 'color:var(--bad)', onclick: function () { if (confirm('Remove ' + p.contact + ' from this hotel?')) call('DELETE', '/admin/hotels/' + hotel.id + '/staff/' + p.user_id).then(reload, function (e) { alert(e.message); }); } }, 'Remove') : null)));
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
      call('POST', '/admin/hotels/' + hotel.id + '/staff', { contact: contact.value, role: isAdmin ? role.value : 'receptionist' }).then(reload, function (err) { btn.disabled = false; add(out, msg('err', err.message)); });
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
      call('PUT', '/admin/hotels/' + hotel.id + '/alerts', { alert_mode: mode.value, escalate_after_min: parseInt(wait.value, 10) || 0 }).then(reload, function (err) { save.disabled = false; add(out, msg('err', err.message)); });
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
        call('PUT', '/admin/hotels/' + hotel.id + '/staff/' + p.user_id + '/alerts', { alerts_on: on.checked, shift_start: from.value, shift_end: to.value })
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

  var PAYOUTS = [['telebirr', 'Telebirr'], ['cbe_birr', 'CBE Birr'], ['bank_transfer', 'Bank transfer']];

  // The business behind the hotel: platform admins only, because it holds the account the hotel is paid into.
  function businessPanel(hotel, reload) {
    var b = hotel.business || { legal_name: '', license_number: '', tin: '', contact_name: '', contact_phone: '', contact_email: '', payout_method: 'bank_transfer', payout_account_name: '', payout_last4: '' };
    var f = {
      legal: h('input', { value: b.legal_name }), license: h('input', { value: b.license_number }), tin: h('input', { value: b.tin || '' }),
      cname: h('input', { value: b.contact_name }), cphone: h('input', { value: b.contact_phone, placeholder: '+251 9…' }), cmail: h('input', { value: b.contact_email, type: 'email' }),
      method: select(PAYOUTS, b.payout_method), aname: h('input', { value: b.payout_account_name }),
      anumber: h('input', { value: '', autocomplete: 'off', placeholder: hotel.business ? 'Leave empty to keep •••• ' + b.payout_last4 : 'Account or phone number' })
    };
    var out = h('div', {});
    var shown = h('div', {});
    var btn = h('button', { type: 'submit' }, 'Save business details');
    var wrap = h('div', {});
    add(wrap, h('p', { class: 'soft' }, 'Who the business is and where BookDan pays it. Required before the hotel can be published. The payout account number is stored encrypted; only its last four digits are shown here.'));
    add(wrap, h('form', { onsubmit: function (e) {
      e.preventDefault(); btn.disabled = true; clear(out);
      call('PUT', '/admin/hotels/' + hotel.id + '/business', {
        legal_name: f.legal.value, license_number: f.license.value, tin: f.tin.value, contact_name: f.cname.value, contact_phone: f.cphone.value, contact_email: f.cmail.value,
        payout_method: f.method.value, payout_account_name: f.aname.value, payout_account_number: f.anumber.value
      }).then(function () { reload(); }, function (err) { btn.disabled = false; add(out, msg('err', err.message)); });
    } },
      h('div', { class: 'grid' }, field('Legal name of the business', f.legal), field('Trade licence number', f.license), field('Tax number (TIN), optional', f.tin),
        field('Contact person', f.cname), field('Contact phone', f.cphone), field('Contact email', f.cmail)),
      h('h3', { style: 'margin-top:20px' }, 'Where BookDan pays this hotel'),
      h('div', { class: 'grid' }, field('Method', f.method), field('Name on the account', f.aname), field('Account or phone number', f.anumber)),
      hotel.business ? h('div', { class: 'field' }, h('button', { type: 'button', class: 'secondary', onclick: function () {
        if (!confirm('Show the full account number? This is recorded in the audit log.')) return;
        call('POST', '/admin/hotels/' + hotel.id + '/business/payout/reveal').then(function (res) { clear(shown); add(shown, msg('ok', 'Account number: ' + res.account_number + ' (this view was logged)')); }, function (e) { clear(shown); add(shown, msg('err', e.message)); });
      } }, 'Show the full account number'), shown) : null,
      h('div', { class: 'field' }, btn), out));
    return wrap;
  }

  // ---- payouts (platform admin): who is owed what, and recording a transfer that was made
  function etb(santim) { return 'ETB ' + (santim / 100).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }); }
  var METHODS = { telebirr: 'Telebirr', cbe_birr: 'CBE Birr', bank_transfer: 'Bank transfer' };

  function payoutsView() {
    var holder = h('div', {}, h('p', { class: 'muted' }, 'Loading…'));
    function load() {
      call('GET', '/admin/payouts').then(function (res) {
        clear(holder);
        if (!res.hotels.length) { add(holder, h('div', { class: 'card' }, h('p', {}, 'No hotel has earned anything yet. Money appears here once a hotel accepts a booking that was paid online.'))); return; }
        add(holder, h('p', { class: 'soft' }, 'To pay is what BookDan owes a hotel today: bookings where the guest has checked in or the stay is over, minus what was already paid. Money for stays still to come is not payable yet, because the guest can still cancel. Make the transfer yourself (bank or Telebirr), then record it here with its reference.'));
        res.hotels.forEach(function (x) { add(holder, payoutCard(x, load)); });
      }, function (err) { clear(holder); add(holder, msg('err', err.message)); });
    }
    load();
    return shell(h('div', {}, h('button', { class: 'ghost', onclick: function () { go('#/'); } }, '← All hotels'), h('h1', {}, 'Payouts'), holder));
  }

  // The statement of one month as a CSV file: fetched with the admin's token and saved by the browser.
  function downloadStatement(hotelId, month, out) {
    clear(out);
    fetch(API + '/admin/hotels/' + hotelId + '/statements?format=csv&month=' + encodeURIComponent(month), { headers: { Authorization: 'Bearer ' + state.token } }).then(function (res) {
      if (!res.ok) return res.json().then(function (j) { throw new Error((j.error && j.error.message) || 'Could not get the statement'); });
      return res.blob();
    }).then(function (blob) {
      var a = document.createElement('a');
      a.href = URL.createObjectURL(blob); a.download = 'bookdan-statement-' + month + '.csv';
      document.body.appendChild(a); a.click(); a.remove();
    }).catch(function (err) { add(out, msg('err', err.message)); });
  }

  function receiptBlock(x, reload) {
    var amount = h('input', { value: String(x.owes_santim / 100), inputmode: 'decimal', placeholder: 'Amount in birr' });
    var ref = h('input', { placeholder: 'Bank transaction reference' });
    var note = h('input', { placeholder: 'Optional note' });
    var out = h('div', {});
    var btn = h('button', { type: 'submit' }, 'Record payment received');
    return h('div', { class: 'card', style: 'background:var(--soft,#fff7e6)' },
      h('h3', {}, 'This hotel owes BookDan ' + etb(x.owes_santim)),
      h('p', { class: 'soft' }, 'The commission on its pay-at-hotel stays is more than BookDan owes it. When the hotel has paid, record the transfer here: it lowers what it owes.'),
      h('form', { onsubmit: function (e) {
        e.preventDefault(); clear(out);
        var santim = Math.round(parseFloat(String(amount.value).replace(/,/g, '')) * 100);
        if (!(santim > 0)) { add(out, msg('err', 'Enter the amount in birr.')); return; }
        if (!confirm('Record ' + etb(santim) + ' received from ' + x.name + '? Make sure the money has really arrived.')) return;
        btn.disabled = true;
        call('POST', '/admin/hotels/' + x.id + '/receipts', { amount_santim: santim, reference: ref.value, note: note.value }).then(reload, function (err) { btn.disabled = false; add(out, msg('err', err.message)); });
      } }, h('div', { class: 'grid' }, field('Amount (birr)', amount), field('Transfer reference', ref), field('Note', note)), h('div', { class: 'field' }, btn), out));
  }

  function statementBlock(x) {
    var now = new Date(); now.setMonth(now.getMonth() - 1);
    var month = h('input', { type: 'month', value: now.getFullYear() + '-' + String(now.getMonth() + 1).padStart(2, '0') });
    var out = h('div', {});
    return h('div', { class: 'field' }, h('label', {}, 'Monthly statement'),
      h('div', { class: 'row' }, month, ' ', h('button', { class: 'secondary', type: 'button', onclick: function () { downloadStatement(x.id, month.value, out); } }, 'Download CSV')), out);
  }

  function payoutCard(x, reload) {
    var amount = h('input', { value: x.to_pay_santim ? String(x.to_pay_santim / 100) : '', inputmode: 'decimal', placeholder: 'Amount in birr' });
    var ref = h('input', { placeholder: 'Bank or Telebirr transaction reference' });
    var note = h('input', { placeholder: 'Optional note' });
    var out = h('div', {});
    var history = h('div', {});
    var btn = h('button', { type: 'submit', disabled: !x.to_pay_santim || !x.has_business }, 'Record payout');
    var card = h('div', { class: 'card' },
      h('div', { class: 'row between' }, h('h3', {}, x.name), x.has_business ? h('span', { class: 'soft' }, METHODS[x.payout_method] + ' · ' + x.payout_account_name + ' ••••' + x.payout_last4) : h('span', { class: 'chip draft' }, 'No business details')),
      h('div', { class: 'grid', style: 'margin-top:12px' },
        field('To pay now', h('strong', {}, etb(x.to_pay_santim))),
        field('After check-in (not payable yet)', h('span', {}, etb(x.upcoming_santim))),
        field('Paid so far', h('span', {}, etb(x.paid_santim) + (x.last_paid_at ? ' · last ' + new Date(x.last_paid_at).toLocaleDateString() : '')))),
      x.has_business ? null : h('p', { class: 'muted' }, 'Add the hotel\'s business and payout details (hotel page, Business tab) before paying it.'),
      x.owes_santim > 0 ? receiptBlock(x, reload) : null,
      statementBlock(x),
      h('form', { onsubmit: function (e) {
        e.preventDefault(); clear(out);
        var santim = Math.round(parseFloat(String(amount.value).replace(/,/g, '')) * 100);
        if (!(santim > 0)) { add(out, msg('err', 'Enter the amount in birr.')); return; }
        if (!confirm('Record a payout of ' + etb(santim) + ' to ' + x.name + '? Make sure the transfer has really been sent.')) return;
        btn.disabled = true;
        call('POST', '/admin/hotels/' + x.id + '/payouts', { amount_santim: santim, reference: ref.value, note: note.value }).then(reload, function (err) { btn.disabled = false; add(out, msg('err', err.message)); });
      } }, h('div', { class: 'grid' }, field('Amount (birr)', amount), field('Transfer reference', ref), field('Note', note)), h('div', { class: 'field' }, btn), out),
      h('button', { class: 'ghost', onclick: function () {
        clear(history);
        call('GET', '/admin/hotels/' + x.id + '/payouts').then(function (res) {
          if (!res.payouts.length) { add(history, h('p', { class: 'muted' }, 'No payouts recorded yet.')); return; }
          var table = h('table', {}, h('thead', {}, h('tr', {}, h('th', {}, 'Date'), h('th', {}, 'Amount'), h('th', {}, 'Method'), h('th', {}, 'Reference'), h('th', {}, 'Note'))));
          var body = h('tbody', {});
          res.payouts.forEach(function (p) { add(body, h('tr', {}, h('td', {}, new Date(p.paid_at).toLocaleString()), h('td', {}, etb(p.amount_santim)), h('td', {}, METHODS[p.method] || p.method), h('td', {}, p.reference), h('td', {}, p.note || ''))); });
          add(table, body); add(history, h('div', { style: 'overflow-x:auto' }, table));
        }, function (err) { add(history, msg('err', err.message)); });
      } }, 'Show payout history'),
      history);
    return card;
  }

  // ---- coupons (platform admin)
  function couponRule(c) {
    var what = c.kind === 'percent' ? (c.value / 100) + '% off' : birr(c.value) + ' ETB off';
    var bits = [what];
    if (c.max_discount_santim) bits.push('at most ' + birr(c.max_discount_santim) + ' ETB');
    if (c.min_subtotal_santim) bits.push('from ' + birr(c.min_subtotal_santim) + ' ETB');
    if (c.first_booking_only) bits.push('first booking only');
    bits.push(c.per_user_limit + ' per guest');
    if (c.max_uses) bits.push(c.max_uses + ' uses in all');
    if (c.starts_on || c.ends_on) bits.push((c.starts_on || '…') + ' to ' + (c.ends_on || '…'));
    bits.push(c.property_name || 'every hotel');
    return bits.join(' · ');
  }

  function couponsView() {
    var holder = h('div', {}, h('p', { class: 'muted' }, 'Loading…'));
    function load() {
      call('GET', '/admin/coupons').then(function (res) {
        clear(holder);
        if (!res.coupons.length) add(holder, h('div', { class: 'card' }, h('p', {}, 'No coupons yet. Create the first one below.')));
        res.coupons.forEach(function (c) {
          var btn = h('button', { class: 'secondary', onclick: function () {
            btn.disabled = true;
            call('PUT', '/admin/coupons/' + c.id, { kind: c.kind, value: c.value, max_discount_santim: c.max_discount_santim, min_subtotal_santim: c.min_subtotal_santim, description: c.description,
              starts_on: c.starts_on || '', ends_on: c.ends_on || '', max_uses: c.max_uses, per_user_limit: c.per_user_limit, first_booking_only: c.first_booking_only, property_id: c.property_id || '', active: !c.active })
              .then(load, function (e) { btn.disabled = false; alert(e.message); });
          } }, c.active ? 'Switch off' : 'Switch on');
          add(holder, h('div', { class: 'card' }, h('div', { class: 'row between' },
            h('div', {}, h('h3', {}, c.code), h('div', { class: 'soft' }, couponRule(c)), h('div', { class: 'muted' }, 'Used ' + c.uses + ' time' + (c.uses === 1 ? '' : 's') + ' · BookDan paid ' + birr(c.discounted_santim) + ' ETB in discounts')),
            h('div', {}, h('span', { class: 'chip ' + (c.active ? 'active' : 'hidden') }, c.active ? 'On' : 'Off'), ' ', btn))));
        });
      }, function (err) { clear(holder); add(holder, msg('err', err.message)); });
    }
    load();

    var f = {
      code: h('input', { placeholder: 'WELCOME10', maxlength: '20', style: 'text-transform:uppercase' }), kind: select([['percent', 'Percent off'], ['fixed', 'Fixed amount off']], 'percent'),
      value: h('input', { inputmode: 'decimal', placeholder: '10 (percent) or 200 (birr)' }), max: h('input', { inputmode: 'decimal', placeholder: 'optional, birr' }),
      min: h('input', { inputmode: 'decimal', placeholder: 'optional, birr' }), starts: h('input', { type: 'date' }), ends: h('input', { type: 'date' }),
      uses: h('input', { inputmode: 'numeric', placeholder: 'optional' }), per: h('input', { inputmode: 'numeric', value: '1' }),
      first: h('input', { type: 'checkbox' }), hotel: h('input', { placeholder: 'optional: a hotel id; empty = every hotel' }), desc: h('input', { placeholder: 'optional note for your team' })
    };
    var out = h('div', {});
    var btn = h('button', { type: 'submit' }, 'Create coupon');
    var form = h('form', { onsubmit: function (e) {
      e.preventDefault(); btn.disabled = true; clear(out);
      var v = parseFloat(String(f.value.value).replace(/,/g, ''));
      var body = {
        code: f.code.value, kind: f.kind.value, value: isNaN(v) ? 0 : (f.kind.value === 'percent' ? Math.round(v * 100) : Math.round(v * 100)),
        max_discount_santim: optionalBirr(f.max.value), min_subtotal_santim: optionalBirr(f.min.value) || 0, description: f.desc.value,
        starts_on: f.starts.value, ends_on: f.ends.value, max_uses: parseInt(f.uses.value, 10) || null, per_user_limit: parseInt(f.per.value, 10) || 1,
        first_booking_only: f.first.checked, property_id: f.hotel.value.trim(), active: true
      };
      call('POST', '/admin/coupons', body).then(function () { btn.disabled = false; load(); }, function (err) { btn.disabled = false; add(out, msg('err', err.message)); });
    } },
      h('div', { class: 'grid' }, field('Code', f.code, 'Letters and digits. Guests type it at checkout.'), field('Type', f.kind), field('Value', f.value, 'Percent: 10 means 10%. Fixed: an amount in birr.'),
        field('Most it can take off (birr)', f.max), field('Smallest booking it applies to (birr)', f.min), field('First day', f.starts), field('Last day', f.ends),
        field('Uses in total', f.uses), field('Uses per guest', f.per), field('Hotel', f.hotel), field('Note', f.desc)),
      h('div', { class: 'field' }, h('label', { class: 'check' }, f.first, 'First booking only')),
      h('div', { class: 'field' }, btn), out);
    return shell(h('div', {}, h('button', { class: 'ghost', onclick: function () { go('#/'); } }, '← All hotels'), h('h1', {}, 'Coupons'),
      h('p', { class: 'soft' }, 'The discount comes off the room price before VAT. BookDan pays for it: the hotel still receives its full share. Coupons work with online payment only.'),
      holder, h('div', { class: 'card' }, h('h3', {}, 'Create a coupon'), form)));
  }

  // ---- payment issues (platform admin): payments a person must look at
  function issuesView() {
    var holder = h('div', {}, h('p', { class: 'muted' }, 'Loading…'));
    function load() {
      call('GET', '/admin/payment-issues').then(function (res) {
        clear(holder);
        if (!res.issues.length) { add(holder, h('div', { class: 'card' }, h('p', {}, 'No payment problems. Every payment matched what was asked.'))); return; }
        res.issues.forEach(function (i) {
          var d = i.details || {};
          var card = h('div', { class: 'card' },
            h('div', { class: 'row between' }, h('h3', {}, i.booking_code + ' · ' + i.hotel),
              h('span', { class: 'chip ' + (i.resolved_at ? 'active' : 'draft') }, i.resolved_at ? 'Resolved' : 'Needs a person')),
            h('p', {}, 'Guest: ' + i.guest_name),
            h('p', {}, 'Asked: ' + etb(d.asked_santim) + ' · the gateway reports: ' + etb(d.paid_santim) + ' ' + (d.paid_currency || '')),
            h('p', { class: 'muted' }, 'Raised ' + when(i.created_at) + (i.resolved_at ? ' · resolved by ' + i.resolved_by + ': ' + i.note : '')));
          if (!i.resolved_at) {
            var note = h('input', { placeholder: 'What did you decide? For example: refunded the guest and called them', maxlength: '1000' });
            var out = h('div', {});
            var btn = h('button', { onclick: function () {
              btn.disabled = true; clear(out);
              call('POST', '/admin/payment-issues/' + i.id + '/resolve', { note: note.value }).then(load, function (err) { btn.disabled = false; add(out, msg('err', err.message)); });
            } }, 'Mark resolved');
            add(card, h('div', { class: 'field' }, h('label', {}, 'Decision'), note), btn, out);
          }
          add(holder, card);
        });
      }, function (err) { clear(holder); add(holder, msg('err', err.message)); });
    }
    load();
    return shell(h('div', {}, h('button', { class: 'ghost', onclick: function () { go('#/'); } }, '← All hotels'), h('h1', {}, 'Payment issues'),
      h('p', { class: 'soft' }, 'A guest paid, but the amount or currency is not what BookDan asked for. The booking is NOT confirmed. Decide with the guest whether to refund or ask for the difference, then write what you decided.'),
      holder));
  }

  // ---- support (platform admin): guests' questions and problems
  var kindLabel = { question: 'Question', problem: 'Problem', dispute: 'Dispute', relocation: 'Hotel cannot host the guest' };
  function supportView() {
    var holder = h('div', {}, h('p', { class: 'muted' }, 'Loading…'));
    call('GET', '/admin/support/tickets').then(function (res) {
      clear(holder);
      if (!res.tickets.length) { add(holder, h('div', { class: 'card' }, h('p', {}, 'No support requests yet.'))); return; }
      res.tickets.forEach(function (t) {
        var label = t.status === 'open' ? 'Needs an answer' : t.status === 'answered' ? 'Waiting for the guest' : 'Closed';
        add(holder, h('div', { class: 'card', style: 'cursor:pointer', onclick: function () { go('#/support/' + t.id); } },
          h('div', { class: 'row between' }, h('h3', {}, t.subject), h('span', { class: 'chip ' + (t.status === 'open' ? 'draft' : 'active') }, label)),
          h('p', { class: 'muted' }, (kindLabel[t.kind] || t.kind) + ' · ' + (t.hotel ? 'hotel staff of ' + t.hotel + ' (' + t.guest + ')' : t.guest) + (t.booking_code ? ' · booking ' + t.booking_code : '') + ' · ' + when(t.updated_at)),
          h('p', {}, t.preview)));
      });
    }, function (err) { clear(holder); add(holder, msg('err', err.message)); });
    return shell(h('div', {}, h('button', { class: 'ghost', onclick: function () { go('#/'); } }, '← All hotels'), h('h1', {}, 'Support'),
      h('p', { class: 'soft' }, 'Questions and problems from guests, newest activity first. Open one to answer it. The guest is emailed your answer.'), holder));
  }

  function ticketView(id) {
    var holder = h('div', {}, h('p', { class: 'muted' }, 'Loading…'));
    function load() {
      call('GET', '/admin/support/tickets/' + id).then(function (res) {
        var t = res.ticket;
        clear(holder);
        add(holder, h('div', { class: 'card' },
          h('div', { class: 'row between' }, h('h3', {}, t.subject), h('span', { class: 'chip ' + (t.status === 'closed' ? 'active' : 'draft') }, t.status)),
          h('p', { class: 'muted' }, (kindLabel[t.kind] || t.kind) + ' · ' + (t.hotel ? 'hotel staff of ' + t.hotel + ' (' + t.guest + ')' : t.guest) + (t.booking_code ? ' · booking ' + t.booking_code : '') + ' · opened ' + when(t.created_at))));
        if (t.booking) {
          var bk = t.booking;
          add(holder, h('div', { class: 'card' },
            h('h3', {}, 'The booking'),
            h('p', {}, bk.hotel + ' · ' + bk.check_in + ' to ' + bk.check_out + ' · status ' + bk.status + (bk.disputed_from ? ' (was ' + bk.disputed_from + ')' : '')),
            h('p', {}, bk.paid_santim ? 'Paid online: ' + etb(bk.paid_santim) + (bk.status === 'CANCELLED_REFUNDED' ? ' (refunded)' : '') : 'Nothing was paid online.'),
            bk.refunded_santim ? h('p', {}, 'Already refunded in part: ' + etb(bk.refunded_santim) + ' · still refundable: ' + etb(bk.left_santim)) : null,
            t.reason ? h('p', { class: 'muted' }, 'Reason chosen by the guest: ' + t.reason) : null,
            t.outcome ? h('p', {}, 'Outcome: ' + t.outcome) : null));
        }
        t.messages.forEach(function (m) {
          add(holder, h('div', { class: 'card', style: m.from_staff ? 'background:var(--soft,#f3f6fb)' : '' },
            h('p', { class: 'muted' }, (m.role === 'hotel' ? 'Hotel (' + m.author.replace(/^hotel:/, '') + ')' : m.role === 'support' ? 'Support (' + m.author + ')' : (t.hotel ? 'Hotel staff' : 'Guest')) + ' · ' + when(m.created_at)),
            h('p', { style: 'white-space:pre-wrap' }, m.body)));
        });
        if (t.status !== 'closed') {
          var box = h('textarea', { rows: '4', maxlength: '2000', placeholder: 'Write your answer' });
          var out = h('div', {});
          var send = h('button', { onclick: function () {
            send.disabled = true; clear(out);
            call('POST', '/admin/support/tickets/' + id + '/reply', { message: box.value }).then(load, function (err) { send.disabled = false; add(out, msg('err', err.message)); });
          } }, 'Send answer');
          var close = (t.kind === 'dispute' || t.kind === 'relocation') ? null : h('button', { class: 'secondary', onclick: function () {
            call('POST', '/admin/support/tickets/' + id + '/close').then(load, function (err) { add(out, msg('err', err.message)); });
          } }, 'Close without answering');
          add(holder, h('div', { class: 'card' }, h('div', { class: 'field' }, h('label', {}, 'Your answer'), box), send, close ? ' ' : null, close, out));
          var partialOK = ['CONFIRMED', 'CHECKED_IN', 'COMPLETED', 'NO_SHOW'].indexOf(t.booking ? t.booking.status : '') >= 0;
          if (t.kind !== 'dispute' && t.booking && partialOK && t.booking.left_santim > 0) {
            var gPayer = h('select', {}, h('option', { value: 'platform' }, 'BookDan pays (the hotel keeps its share)'), h('option', { value: 'hotel' }, 'The hotel pays its share of it'));
            var gAmount = h('input', { type: 'number', step: '0.01', min: '0.01', placeholder: 'Amount in birr' });
            var gWhy = h('textarea', { rows: '2', maxlength: '1000', placeholder: 'The reason. The guest reads it.' });
            var gOut = h('div', {});
            var gBtn = h('button', { class: 'secondary', onclick: function () {
              if (!confirm('Refund part of the payment to the guest? This cannot be undone.')) return;
              gBtn.disabled = true; clear(gOut);
              call('POST', '/admin/support/tickets/' + id + '/refund', { payer: gPayer.value, amount_santim: Math.round(parseFloat(gAmount.value) * 100), note: gWhy.value }).then(load, function (err) { gBtn.disabled = false; add(gOut, msg('err', err.message)); });
            } }, 'Refund this part');
            add(holder, h('div', { class: 'card' }, h('h3', {}, 'Refund part of the payment'),
              h('p', { class: 'soft' }, 'A goodwill credit: the booking stands. You can refund at most ' + etb(t.booking.left_santim) + ' (less than the whole payment).'),
              h('div', { class: 'field' }, h('label', {}, 'Who pays'), gPayer), h('div', { class: 'field' }, h('label', {}, 'Amount'), gAmount), h('div', { class: 'field' }, h('label', {}, 'Reason'), gWhy), gBtn, gOut));
          }
          if (t.kind === 'relocation') {
            var rOutcome = h('select', {},
              h('option', { value: 'restored' }, 'Restore: the hotel can host the guest after all (the booking is confirmed again)'),
              h('option', { value: 'refunded' }, 'Refund in full: no other place was found (the hotel bears the refund)'));
            var rWhy = h('textarea', { rows: '3', maxlength: '1000', placeholder: 'What was decided. The guest reads it.' });
            var rOut = h('div', {});
            var rBtn = h('button', { onclick: function () {
              if (!confirm('Settle this relocation? A refund cannot be undone.')) return;
              rBtn.disabled = true; clear(rOut);
              call('POST', '/admin/support/tickets/' + id + '/relocation', { outcome: rOutcome.value, note: rWhy.value }).then(load, function (err) { rBtn.disabled = false; add(rOut, msg('err', err.message)); });
            } }, 'Settle the relocation');
            add(holder, h('div', { class: 'card' }, h('h3', {}, 'Settle the relocation'),
              h('p', { class: 'soft' }, 'The hotel told BookDan it cannot host this guest. Find the guest another place and tell them in the answer box above, then settle it here. The guest can also cancel for a full refund at any time.'),
              h('div', { class: 'field' }, h('label', {}, 'Outcome'), rOutcome), h('div', { class: 'field' }, h('label', {}, 'Reason'), rWhy), rBtn, rOut));
          }
          if (t.kind === 'dispute') {
            var paid = t.booking && t.booking.paid_santim > 0;
            var outcome = h('select', {},
              h('option', { value: 'rejected' }, 'Reject: nothing changes, the booking goes back to what it was'),
              paid ? h('option', { value: 'refunded_hotel_pays' }, 'Refund in full: the hotel pays (its share and our commission are reversed)') : null,
              paid ? h('option', { value: 'refunded_platform_pays' }, 'Refund in full: BookDan pays (the hotel keeps its share)') : null,
              paid ? h('option', { value: 'refunded_partial_hotel_pays' }, 'Refund part: the hotel pays its share of it, the booking stands') : null,
              paid ? h('option', { value: 'refunded_partial_platform_pays' }, 'Refund part: BookDan pays, the booking stands') : null);
            var amountBox = h('input', { type: 'number', step: '0.01', min: '0.01', placeholder: 'Amount in birr (for a part refund)' });
            var why = h('textarea', { rows: '3', maxlength: '1000', placeholder: 'The reason for your decision. The guest reads it.' });
            var dout = h('div', {});
            var decide = h('button', { onclick: function () {
              if (!confirm('Settle this dispute? This closes it, and a refund cannot be undone.')) return;
              decide.disabled = true; clear(dout);
              var body = { outcome: outcome.value, note: why.value };
              if (outcome.value.indexOf('refunded_partial_') === 0) body.amount_santim = Math.round(parseFloat(amountBox.value) * 100);
              call('POST', '/admin/support/tickets/' + id + '/resolve', body).then(load, function (err) { decide.disabled = false; add(dout, msg('err', err.message)); });
            } }, 'Settle the dispute');
            add(holder, h('div', { class: 'card' }, h('h3', {}, 'Decision'),
              h('p', { class: 'soft' }, 'The booking stays frozen (no money is released to the hotel) until you decide. A part refund returns part of the payment and the booking stands.'),
              h('div', { class: 'field' }, h('label', {}, 'Outcome'), outcome), h('div', { class: 'field' }, h('label', {}, 'Amount to refund (part refunds only)'), amountBox), h('div', { class: 'field' }, h('label', {}, 'Reason'), why), decide, dout));
          }
        }
      }, function (err) { clear(holder); add(holder, msg('err', err.message)); });
    }
    load();
    return shell(h('div', {}, h('button', { class: 'ghost', onclick: function () { go('#/support'); } }, '← All requests'), holder));
  }

  // ---- hotel changes (super admin): what hotels' owners changed through the hotel portal, and what BookDan staff changed
  function activityView() {
    var holder = h('div', {}, h('p', { class: 'muted' }, 'Loading…'));
    var only = h('input', { type: 'checkbox', checked: true });
    function load() {
      call('GET', '/admin/activity?limit=200' + (only.checked ? '&by=hotel' : '')).then(function (res) {
        clear(holder);
        if (!res.items.length) { add(holder, h('div', { class: 'card' }, h('p', { class: 'muted' }, 'Nothing yet.'))); return; }
        var table = h('table', {}, h('thead', {}, h('tr', {}, ['When', 'Hotel', 'Who', 'What'].map(function (t) { return h('th', {}, t); }))));
        var body = h('tbody', {});
        res.items.forEach(function (a) {
          var d = a.details || {};
          var extra = [];
          if (d.name) extra.push(d.name);
          if (d.price_santim) extra.push('ETB ' + Math.round(d.price_santim / 100));
          if (d.status && a.action === 'admin_hotel_updated') extra.push('status ' + d.status);
          add(body, h('tr', {}, h('td', {}, new Date(a.at).toLocaleString()), h('td', {}, a.hotel ? h('a', { href: '#/hotel/' + a.property_id }, a.hotel) : ''),
            h('td', {}, a.who, ' ', h('span', { class: 'chip ' + (a.by_hotel ? 'draft' : 'active') }, a.by_hotel ? 'Hotel' : 'BookDan')), h('td', {}, a.description || a.action, extra.length ? ' (' + extra.join(', ') + ')' : '')));
        });
        add(table, body);
        add(holder, h('div', { class: 'card' }, h('div', { style: 'overflow-x:auto' }, table)));
      }, function (err) { clear(holder); add(holder, msg('err', err.message)); });
    }
    only.addEventListener('change', load);
    load();
    return shell(h('div', {}, h('button', { class: 'ghost', onclick: function () { go('#/'); } }, '← All hotels'), h('h1', {}, 'Hotel changes'),
      h('p', { class: 'soft' }, 'Hotels change their own rooms, prices, photos and team in the hotel portal and it goes live at once. This is the record of those changes.'),
      h('label', { class: 'check' }, only, 'Show only changes made by hotels'), holder));
  }

  // ---- exchange rates (platform admin): USD and EUR shown next to ETB prices
  function ratesView() {
    var holder = h('div', {}, h('p', { class: 'muted' }, 'Loading…'));
    var note = h('div', {});
    var force = h('input', { type: 'checkbox' });
    function load() {
      call('GET', '/admin/rates').then(function (res) {
        clear(holder);
        var byCur = {};
        res.rates.forEach(function (r) { byCur[r.currency] = r; });
        if (res.feed) {
          add(holder, h('div', { class: 'card' },
            h('h3', {}, 'Fetch current rates'),
            h('p', { class: 'soft' }, 'Fills every currency nobody typed (and any rate that went out of date) from a public exchange-rate feed. A rate you typed yourself is left alone, unless you tick the box. This also happens by itself once a day. Rates by ExchangeRate-API.'),
            h('label', { class: 'row' }, force, ' Replace the rates I typed as well'),
            h('button', { onclick: function () {
              clear(note);
              call('POST', '/admin/rates/refresh', { force: force.checked }).then(function (r) {
                add(note, msg('ok', r.written.length + ' rate' + (r.written.length === 1 ? '' : 's') + ' updated' + (r.left_alone.length ? ', ' + r.left_alone.length + ' typed rate' + (r.left_alone.length === 1 ? '' : 's') + ' left as they were' : '') + '.'));
                load();
              }, function (err) { add(note, msg('err', err.message)); });
            } }, 'Fetch current rates'), note));
        } else {
          add(holder, h('div', { class: 'card' }, h('p', { class: 'soft' }, 'The exchange-rate feed is switched off on this server (FX_FEED_URL is empty), so every rate is typed by hand.')));
        }
        var table = h('table', {}, h('thead', {}, h('tr', {}, h('th', {}, 'Currency'), h('th', {}, 'Birr for 1 unit'), h('th', {}, 'Status'), h('th', {}, 'Set'), h('th', {}, ''))));
        var body = h('tbody', {});
        res.supported.forEach(function (cur) {
          var r = byCur[cur];
          var input = h('input', { type: 'number', step: '0.0001', min: '0.01', placeholder: 'Birr for 1 ' + cur, value: r ? String(r.birr_per_unit) : '', style: 'width:130px' });
          var out = h('div', {});
          function save(confirmed) {
            clear(out);
            call('PUT', '/admin/rates/' + cur, { birr_per_unit: parseFloat(input.value), confirm: confirmed === true }).then(load, function (err) {
              if (err.code === 'RATE_JUMP' && confirm(err.message + '\n\nSave it anyway?')) return save(true);
              add(out, msg('err', err.message));
            });
          }
          var status = !r ? h('span', { class: 'chip draft' }, 'Not set') : r.stale ? h('span', { class: 'chip draft' }, 'Out of date') : h('span', { class: 'chip active' }, 'Shown to guests');
          var by = r ? (r.updated_by && r.updated_by.indexOf('feed:') === 0 ? 'feed, ' : 'typed, ') + when(r.updated_at) : '';
          add(body, h('tr', {}, h('td', {}, h('strong', {}, cur)), h('td', {}, input), h('td', {}, status), h('td', { class: 'soft' }, by), h('td', {}, h('button', { class: 'secondary', onclick: function () { save(false); } }, 'Save'), out)));
        });
        add(table, body);
        add(holder, h('div', { class: 'card', style: 'overflow-x:auto' }, table));
      }, function (err) { clear(holder); add(holder, msg('err', err.message)); });
    }
    load();
    return shell(h('div', {}, h('button', { class: 'ghost', onclick: function () { go('#/'); } }, '← All hotels'), h('h1', {}, 'Exchange rates'),
      h('p', { class: 'soft' }, 'Guests can choose to see prices in about thirty currencies (dollars, euros, pounds, dirhams, riyals and more) next to the birr price. It is only a guide: BookDan always charges and refunds in birr. A rate you do not update for 30 days stops being shown, and the currency leaves the guests\' list.'),
      holder));
  }

  // ---- calls (platform admin): how often guests tap "Call the hotel" on a confirmed booking
  function callsView() {
    var holder = h('div', {}, h('p', { class: 'muted' }, 'Loading…'));
    var days = h('select', {}, h('option', { value: '7' }, 'Last 7 days'), h('option', { value: '30', selected: 'selected' }, 'Last 30 days'), h('option', { value: '90' }, 'Last 90 days'));
    function load() {
      call('GET', '/admin/calls?days=' + days.value).then(function (res) {
        clear(holder);
        if (!res.hotels.length) { add(holder, h('div', { class: 'card' }, h('p', {}, 'No calls were logged in this period.'))); return; }
        var table = h('table', {}, h('thead', {}, h('tr', {}, h('th', {}, 'Hotel'), h('th', {}, 'Calls'), h('th', {}, 'Bookings with a call'), h('th', {}, 'Confirmed bookings'), h('th', {}, 'Last call'))));
        var body = h('tbody', {});
        res.hotels.forEach(function (x) {
          add(body, h('tr', {}, h('td', {}, x.name), h('td', {}, String(x.calls)), h('td', {}, String(x.bookings_with_calls)), h('td', {}, String(x.confirmed_bookings)), h('td', {}, x.last_call_at ? new Date(x.last_call_at).toLocaleString() : '')));
        });
        add(table, body);
        var rt = h('table', {}, h('thead', {}, h('tr', {}, h('th', {}, 'When'), h('th', {}, 'Hotel'), h('th', {}, 'Booking'), h('th', {}, 'Guest'), h('th', {}, 'Status'))));
        var rb = h('tbody', {});
        res.recent.forEach(function (c) { add(rb, h('tr', {}, h('td', {}, new Date(c.at).toLocaleString()), h('td', {}, c.hotel), h('td', {}, c.booking_code), h('td', {}, c.guest_name), h('td', {}, c.status))); });
        add(rt, rb);
        add(holder, h('div', { class: 'card', style: 'overflow-x:auto' }, h('h3', {}, 'By hotel'), table), h('div', { class: 'card', style: 'overflow-x:auto' }, h('h3', {}, 'Latest calls'), rt));
      }, function (err) { clear(holder); add(holder, msg('err', err.message)); });
    }
    days.onchange = load;
    load();
    return shell(h('div', {}, h('button', { class: 'ghost', onclick: function () { go('#/'); } }, '← All hotels'), h('h1', {}, 'Calls'),
      h('p', { class: 'soft' }, 'A guest sees the hotel\'s number only once a booking is confirmed, and the app tells BookDan when they tap "Call the hotel". A hotel whose guests call it far more often than others may be arranging bookings outside BookDan. A tap is not proof of a call.'),
      h('div', { class: 'field' }, days), holder));
  }

  // ---- my account (every platform admin): my password and the authenticator app
  function accountView(forced) {
    var holder = h('div', {});
    function paint() {
      clear(holder);
      if (!state.hasAccount) {
        add(holder, h('div', { class: 'card' }, h('h3', {}, 'You are using the shared setup password'),
          h('p', { class: 'soft' }, 'It has no password or authenticator app of its own. A super admin can create your own account in the Admins tab, using your email address. After that only your own password works for you, and the shared one can be removed from the server settings.')));
        return;
      }
      var cur = h('input', { type: 'password', autocomplete: 'current-password' });
      var nw = h('input', { type: 'password', autocomplete: 'new-password' });
      var out = h('div', {});
      add(holder, h('div', { class: 'card' }, h('h3', {}, forced ? 'Choose your own password' : 'Change my password'),
        forced ? h('p', { class: 'soft' }, 'The password you were given was only temporary. Choose one that only you know: at least 12 characters, not your email name.') : null,
        h('div', { class: 'field' }, h('label', {}, 'Current password'), cur),
        h('div', { class: 'field' }, h('label', {}, 'New password'), nw),
        h('button', { onclick: function () {
          clear(out);
          call('POST', '/admin/me/password', { current_password: cur.value, new_password: nw.value }).then(function (res) {
            signedIn(res);
          }, function (err) { add(out, msg('err', err.message)); });
        } }, 'Save password'), out));
      if (forced) return;
      var mfaOut = h('div', {});
      var box = h('div', { class: 'card' }, h('h3', {}, 'Authenticator app (second step)'));
      add(box, h('p', { class: 'soft' }, state.mfa
        ? 'On. Signing in needs the 6-digit code from your authenticator app as well as the password.'
        : 'Off. Add an authenticator app (Google Authenticator, Microsoft Authenticator, Authy…) so a stolen password is not enough to get in.'));
      if (state.mfa) {
        var pw = h('input', { type: 'password', autocomplete: 'current-password' });
        var code = h('input', { type: 'text', inputmode: 'numeric', maxlength: '6', placeholder: '6-digit code' });
        add(box, h('div', { class: 'field' }, h('label', {}, 'Password'), pw), h('div', { class: 'field' }, h('label', {}, 'A current code (wait for a new one if you just signed in)'), code),
          h('button', { class: 'secondary', onclick: function () {
            clear(mfaOut);
            call('POST', '/admin/me/mfa/disable', { password: pw.value, code: code.value }).then(function () { state.mfa = false; paint(); }, function (err) { add(mfaOut, msg('err', err.message)); });
          } }, 'Turn it off'), mfaOut);
      } else {
        add(box, h('button', { onclick: function () {
          clear(mfaOut);
          call('POST', '/admin/me/mfa/setup').then(function (res) {
            var code2 = h('input', { type: 'text', inputmode: 'numeric', maxlength: '6', placeholder: '6-digit code' });
            clear(mfaOut);
            add(mfaOut, h('p', {}, 'In your authenticator app choose "Enter a setup key" and type this key (account: BookDan, type: time based):'),
              h('p', {}, h('code', { style: 'font-size:16px;word-break:break-all' }, res.secret)),
              h('div', { class: 'field' }, h('label', {}, 'Then type the 6-digit code the app shows'), code2),
              h('button', { onclick: function () {
                call('POST', '/admin/me/mfa/enable', { code: code2.value }).then(function () { state.mfa = true; paint(); }, function (err) { add(mfaOut, msg('err', err.message)); });
              } }, 'Confirm and turn on'));
          }, function (err) { add(mfaOut, msg('err', err.message)); });
        } }, 'Set up'), mfaOut);
      }
      add(holder, box);
    }
    paint();
    return shell(h('div', {}, forced ? null : h('button', { class: 'ghost', onclick: function () { go('#/'); } }, '← All hotels'), h('h1', {}, 'My account'), holder));
  }

  // ---- admins (super admin): who can use this page, and what they may do
  function adminsView() {
    var holder = h('div', { class: 'card', style: 'overflow-x:auto' }, h('p', { class: 'muted' }, 'Loading…'));
    var notice = h('div', {});
    var ROLES = { super: 'Super admin', support: 'Support', finance: 'Finance' };
    function shown(title, password) {
      clear(notice);
      add(notice, h('div', { class: 'msg ok', role: 'status' }, title, ' ', h('code', { style: 'font-size:16px;user-select:all' }, password),
        h('div', { class: 'soft' }, 'Copy it now: it is shown only once. They must choose their own password the first time they sign in.')));
    }
    function load() {
      call('GET', '/admin/admins').then(function (res) {
        clear(holder);
        if (!res.admins.length) { add(holder, h('p', {}, 'No admin accounts yet. You are signed in with the shared setup password. Create your own account first, using your email address.')); return; }
        var table = h('table', {}, h('thead', {}, h('tr', {}, h('th', {}, 'Email'), h('th', {}, 'Role'), h('th', {}, 'Status'), h('th', {}, 'Authenticator'), h('th', {}, 'Last sign-in'), h('th', {}, ''))));
        var body = h('tbody', {});
        res.admins.forEach(function (a) {
          var role = h('select', { onchange: function () {
            call('PUT', '/admin/admins/' + a.id, { role: role.value }).then(load, function (e) { alert(e.message); load(); });
          } });
          Object.keys(ROLES).forEach(function (k) { add(role, h('option', { value: k, selected: a.role === k ? 'selected' : null }, ROLES[k])); });
          var status = a.disabled ? h('span', { class: 'chip hidden' }, 'Disabled') : a.locked ? h('span', { class: 'chip draft' }, 'Locked') : a.must_change_password ? h('span', { class: 'chip draft' }, 'Must set a password') : h('span', { class: 'chip active' }, 'Active');
          var actions = h('span', {},
            h('button', { class: 'ghost', onclick: function () { call('PUT', '/admin/admins/' + a.id, { disabled: !a.disabled }).then(load, function (e) { alert(e.message); }); } }, a.disabled ? 'Enable' : 'Disable'), ' ',
            a.locked ? h('button', { class: 'ghost', onclick: function () { call('PUT', '/admin/admins/' + a.id, { unlock: true }).then(load, function (e) { alert(e.message); }); } }, 'Unlock') : null, ' ',
            h('button', { class: 'ghost', onclick: function () {
              if (confirm('Give ' + a.email + ' a new temporary password? Their current sessions end.')) call('POST', '/admin/admins/' + a.id + '/reset-password').then(function (r) { shown('New password for ' + a.email + ':', r.temporary_password); load(); }, function (e) { alert(e.message); });
            } }, 'Reset password'), ' ',
            a.mfa ? h('button', { class: 'ghost', onclick: function () {
              if (confirm('Remove the authenticator app of ' + a.email + '? Use this when they lost their phone.')) call('POST', '/admin/admins/' + a.id + '/reset-mfa').then(load, function (e) { alert(e.message); });
            } }, 'Remove authenticator') : null);
          add(body, h('tr', {}, h('td', {}, a.email), h('td', {}, role), h('td', {}, status), h('td', {}, a.mfa ? 'On' : 'Off'), h('td', {}, a.last_login_at ? new Date(a.last_login_at).toLocaleString() : 'Never'), h('td', {}, actions)));
        });
        add(table, body);
        add(holder, table);
      }, function (err) { clear(holder); add(holder, msg('err', err.message)); });
    }
    var email = h('input', { type: 'email', placeholder: 'name@example.com' });
    var role = h('select', {});
    Object.keys(ROLES).forEach(function (k) { add(role, h('option', { value: k }, ROLES[k] + (k === 'super' ? ' (everything)' : k === 'support' ? ' (support queue, reviews)' : ' (payouts, statements, rates)'))); });
    var out = h('div', {});
    var create = h('div', { class: 'card' }, h('h3', {}, 'Add an admin'),
      h('div', { class: 'grid' }, h('div', { class: 'field' }, h('label', {}, 'Email'), email), h('div', { class: 'field' }, h('label', {}, 'Role'), role)),
      h('p', { class: 'soft' }, 'A temporary password is generated and shown once. Send it to them by a private channel. They choose their own at the first sign-in.'),
      h('button', { onclick: function () {
        clear(out);
        call('POST', '/admin/admins', { email: email.value, role: role.value }).then(function (r) { email.value = ''; shown('Temporary password for ' + r.email + ':', r.temporary_password); load(); }, function (err) { add(out, msg('err', err.message)); });
      } }, 'Add admin'), out);
    load();
    return shell(h('div', {}, h('button', { class: 'ghost', onclick: function () { go('#/'); } }, '← All hotels'), h('h1', {}, 'Admins'),
      h('p', { class: 'soft' }, 'Everyone who works at BookDan has their own sign-in. Super admins can do everything; Support answers guests and hotels, settles disputes and removes abusive reviews; Finance handles payouts, statements, payment issues and exchange rates. A disabled admin is signed out at once, and five wrong passwords lock an account for 15 minutes.'),
      notice, create, holder));
  }

  // ---- reviews (platform admin): take down an abusive review, bring it back
  function reviewsView() {
    var holder = h('div', {}, h('p', { class: 'muted' }, 'Loading…'));
    var status = h('select', {}, h('option', { value: 'visible' }, 'Shown to guests'), h('option', { value: 'hidden' }, 'Hidden'), h('option', { value: 'all' }, 'All'));
    var sort = h('select', {}, h('option', { value: 'latest' }, 'Newest first'), h('option', { value: 'lowest' }, 'Lowest rating first'));
    var words = h('input', { type: 'search', placeholder: 'Words or booking code' });
    function load() {
      call('GET', '/admin/reviews?status=' + status.value + '&sort=' + sort.value + '&q=' + encodeURIComponent(words.value.trim())).then(function (res) {
        clear(holder);
        if (!res.reviews.length) { add(holder, h('div', { class: 'card' }, h('p', {}, 'No reviews match.'))); return; }
        res.reviews.forEach(function (v) {
          var actions = v.hidden
            ? h('button', { class: 'secondary', onclick: function () { call('POST', '/admin/reviews/' + v.id + '/restore').then(load, function (e) { alert(e.message); }); } }, 'Show again')
            : h('button', { class: 'ghost', style: 'color:var(--bad)', onclick: function () {
                var reason = prompt('Why is this review being hidden? (abusive, spam, private details…)');
                if (reason === null) return;
                call('POST', '/admin/reviews/' + v.id + '/hide', { reason: reason }).then(load, function (e) { alert(e.message); });
              } }, 'Hide');
          add(holder, h('div', { class: 'card' },
            h('div', { class: 'row' }, h('strong', {}, v.hotel), ' ', h('span', { class: 'muted' }, '★'.repeat(v.rating) + ' · ' + v.author + ' · ' + v.booking_code + ' · ' + v.date), v.hidden ? h('span', { class: 'chip', style: 'color:var(--bad)' }, 'Hidden') : null),
            h('p', {}, v.text || '(no comment)'),
            v.reply ? h('p', { class: 'soft' }, 'Hotel reply: ' + v.reply) : null,
            v.hidden ? h('p', { class: 'soft' }, 'Hidden' + (v.hidden_by ? ' by ' + v.hidden_by : '') + ': ' + v.hidden_reason) : null,
            actions));
        });
      }, function (err) { clear(holder); add(holder, msg('err', err.message)); });
    }
    status.onchange = load; sort.onchange = load;
    words.onkeydown = function (e) { if (e.key === 'Enter') load(); };
    load();
    return shell(h('div', {}, h('button', { class: 'ghost', onclick: function () { go('#/'); } }, '← All hotels'), h('h1', {}, 'Reviews'),
      h('p', { class: 'soft' }, 'Hide a review that is abusive, spam or shows private details. A hidden review disappears from the hotel page and its rating, and the hotel can no longer answer it; the guest who wrote it still sees it. Every change is recorded with your reason, and you can show it again.'),
      h('div', { class: 'field' }, status, ' ', sort, ' ', words), holder));
  }

  // ---- analytics (platform admin)
  function pct(a, b) { return b ? Math.round((a / b) * 100) + '%' : '–'; }

  function stat(label, value, hint) {
    return h('div', { class: 'card', style: 'flex:1;min-width:150px;margin:6px' }, h('div', { class: 'muted' }, label), h('div', { style: 'font-size:1.5rem;font-weight:700;margin-top:2px' }, value), hint ? h('div', { class: 'muted' }, hint) : null);
  }

  // A column chart made of plain boxes: one column per day, its height the value against the busiest day.
  function bars(points, key, label) {
    var max = 0;
    points.forEach(function (p) { if (p[key] > max) max = p[key]; });
    var row = h('div', { style: 'display:flex;align-items:flex-end;gap:2px;height:120px;margin-top:8px' });
    points.forEach(function (p) {
      var height = max ? Math.max(2, Math.round((p[key] / max) * 116)) : 2;
      add(row, h('div', { title: p.day + ': ' + label(p[key]), style: 'flex:1;background:var(--accent,#1f4fa0);height:' + height + 'px;border-radius:2px 2px 0 0;opacity:' + (p[key] ? '1' : '.25') }));
    });
    return row;
  }

  function analyticsView() {
    var holder = h('div', {}, h('p', { class: 'muted' }, 'Loading…'));
    var days = h('select', {}, h('option', { value: '7' }, 'Last 7 days'), h('option', { value: '30', selected: 'selected' }, 'Last 30 days'), h('option', { value: '90' }, 'Last 90 days'));
    function load() {
      call('GET', '/admin/analytics?days=' + days.value).then(function (a) {
        clear(holder);
        var attention = [];
        if (a.open_payment_issues) attention.push(['Payment issues', a.open_payment_issues, '#/issues']);
        if (a.open_disputes) attention.push(['Disputes', a.open_disputes, '#/support']);
        if (a.open_relocations) attention.push(['Guests to move', a.open_relocations, '#/support']);
        if (a.open_support_requests) attention.push(['Support requests', a.open_support_requests, '#/support']);
        add(holder,
          h('div', { class: 'card' }, h('h3', {}, 'Needs a person now'),
            attention.length ? h('div', { class: 'row' }, attention.map(function (x) { return h('button', { class: 'secondary', onclick: function () { go(x[2]); } }, x[0] + ': ' + x[1]); }))
              : h('p', {}, 'Nothing is waiting. ' + a.awaiting_hotel_now + ' booking(s) wait for a hotel to answer.'),
            attention.length ? h('p', { class: 'muted' }, a.awaiting_hotel_now + ' booking(s) wait for a hotel to answer.') : null),
          h('div', { style: 'display:flex;flex-wrap:wrap' },
            stat('Booking value (GMV)', etb(a.gmv_santim), 'confirmed by hotels'),
            stat('BookDan commission', etb(a.commission_santim), '+ VAT ' + etb(a.commission_vat_santim)),
            stat('Refunded to guests', etb(a.refunded_santim)),
            stat('Average booking', etb(a.avg_booking_santim), a.avg_nights.toFixed(1) + ' nights')),
          h('div', { class: 'card' }, h('h3', {}, 'From price to stay'),
            h('table', {}, h('tbody', {},
              h('tr', {}, h('td', {}, 'Prices asked for'), h('td', {}, String(a.quotes))),
              h('tr', {}, h('td', {}, 'Rooms held'), h('td', {}, a.holds + ' (' + pct(a.holds, a.quotes) + ' of prices)')),
              h('tr', {}, h('td', {}, 'Paid, or chose to pay at the hotel'), h('td', {}, a.paid + ' (' + pct(a.paid, a.holds) + ' of holds)')),
              h('tr', {}, h('td', {}, 'Confirmed by the hotel'), h('td', {}, a.confirmed + ' (' + pct(a.confirmed, a.paid) + ' of paid)')),
              h('tr', {}, h('td', {}, 'Stays completed'), h('td', {}, String(a.completed))))),
            h('p', { class: 'muted' }, 'Paid online: ' + a.confirmed_paid_online + ' · pay at hotel: ' + a.confirmed_pay_at_hotel + ' · new guests: ' + a.new_guests + ' · returning guests: ' + a.repeat_guests)),
          h('div', { class: 'card' }, h('h3', {}, 'Where bookings ended without a stay'),
            h('table', {}, h('tbody', {},
              h('tr', {}, h('td', {}, 'Hold ran out (not paid)'), h('td', {}, String(a.expired))),
              h('tr', {}, h('td', {}, 'Hotel declined'), h('td', {}, String(a.declined))),
              h('tr', {}, h('td', {}, 'Hotel did not answer in time'), h('td', {}, String(a.no_response))),
              h('tr', {}, h('td', {}, 'Cancelled by the guest'), h('td', {}, String(a.cancelled_by_guest))),
              h('tr', {}, h('td', {}, 'Cancelled by the hotel'), h('td', {}, String(a.cancelled_by_hotel))),
              h('tr', {}, h('td', {}, 'Cancelled by BookDan'), h('td', {}, String(a.cancelled_by_admin))),
              h('tr', {}, h('td', {}, 'Guest did not come'), h('td', {}, String(a.no_shows)))))),
          h('div', { class: 'card' }, h('h3', {}, 'Booking value per day'), bars(a.daily, 'gmv_santim', etb),
            h('div', { class: 'row between muted' }, h('span', {}, a.from), h('span', {}, a.to))),
          h('div', { class: 'card' }, h('h3', {}, 'Bookings confirmed per day'), bars(a.daily, 'confirmed', String),
            h('div', { class: 'row between muted' }, h('span', {}, a.from), h('span', {}, a.to))));
        var table = h('table', {}, h('thead', {}, h('tr', {}, h('th', {}, 'Hotel'), h('th', {}, 'Confirmed'), h('th', {}, 'Value'), h('th', {}, 'Requests'), h('th', {}, 'Accepted'), h('th', {}, 'Declined'), h('th', {}, 'No answer'), h('th', {}, 'Average reply'), h('th', {}, 'It cancelled'))));
        var body = h('tbody', {});
        a.hotels.forEach(function (x) {
          add(body, h('tr', {}, h('td', {}, x.name), h('td', {}, String(x.confirmed)), h('td', {}, etb(x.gmv_santim)), h('td', {}, String(x.requests)), h('td', {}, String(x.accepted)), h('td', {}, String(x.declined)),
            h('td', { style: x.no_response ? 'color:var(--bad,#b00020);font-weight:700' : '' }, String(x.no_response)),
            h('td', {}, x.avg_reply_minutes === null ? '–' : x.avg_reply_minutes.toFixed(1) + ' min'),
            h('td', { style: x.hotel_cancellations ? 'color:var(--bad,#b00020);font-weight:700' : '' }, String(x.hotel_cancellations))));
        });
        add(table, body);
        add(holder, h('div', { class: 'card', style: 'overflow-x:auto' }, h('h3', {}, 'Hotels'), table,
          h('p', { class: 'muted' }, 'A request is a booking that was paid and waited for the hotel in this period. Hotels that often do not answer, or that cancel confirmed bookings, cost BookDan guests.')));
      }, function (err) { clear(holder); add(holder, msg('err', err.message)); });
    }
    days.onchange = load;
    load();
    return shell(h('div', {}, h('button', { class: 'ghost', onclick: function () { go('#/'); } }, '← All hotels'), h('h1', {}, 'Analytics'),
      h('p', { class: 'soft' }, 'The state of the marketplace over the chosen period (Addis Ababa days). Amounts are in birr.'), h('div', { class: 'field' }, days), holder));
  }

  // ---- hotel applications (platform admin): hotels that applied on the public /apply page
  function applicationsView() {
    var holder = h('div', {}, h('p', { class: 'muted' }, 'Loading…'));
    function load() {
      call('GET', '/admin/applications').then(function (res) {
        clear(holder);
        if (!res.applications.length) { add(holder, h('div', { class: 'card' }, h('p', {}, 'No hotel has applied yet. Hotels apply at /apply on this server.'))); return; }
        res.applications.forEach(function (a) {
          var card = h('div', { class: 'card' },
            h('div', { class: 'row between' }, h('h3', {}, a.hotel_name + ' · ' + a.reference),
              h('span', { class: 'chip ' + (a.status === 'pending' ? 'draft' : a.status === 'approved' ? 'active' : 'hidden') }, a.status === 'pending' ? 'Waiting for a decision' : a.status)),
            h('p', {}, a.hotel_type + ' · ' + a.area + ', ' + a.city + ' · ' + a.rooms_count + ' rooms' + (a.pay_at_hotel ? ' · wants pay at hotel' : '')),
            h('p', { class: 'soft' }, a.address + ' · ' + a.hotel_phone),
            a.description ? h('p', {}, a.description) : null,
            h('p', {}, 'Owner: ' + a.owner_name + ' · ' + a.owner_email + ' · ' + a.owner_phone),
            h('p', {}, 'Business: ' + a.legal_name + ' · licence ' + a.license_number),
            h('p', { class: 'muted' }, 'Applied ' + when(a.created_at) + (a.decided_at ? ' · decided ' + when(a.decided_at) + ' by ' + a.decided_by + (a.decision_note ? ': ' + a.decision_note : '') : '')));
          if (a.property_id) add(card, h('button', { class: 'secondary', onclick: function () { go('#/hotel/' + a.property_id); } }, 'Open the hotel'));
          if (a.status === 'pending') {
            var out = h('div', {});
            var note = h('input', { placeholder: 'If you reject it: the reason the applicant will read', maxlength: '1000' });
            var yes = h('button', { onclick: function () {
              if (!confirm('Approve ' + a.hotel_name + '? A draft hotel is created and ' + a.owner_email + ' is invited as its owner.')) return;
              yes.disabled = true; clear(out);
              call('POST', '/admin/applications/' + a.id + '/approve').then(function (r) { go('#/hotel/' + r.property_id); }, function (err) { yes.disabled = false; add(out, msg('err', err.message)); });
            } }, 'Approve and create the hotel');
            var no = h('button', { class: 'secondary', onclick: function () {
              clear(out); no.disabled = true;
              call('POST', '/admin/applications/' + a.id + '/reject', { note: note.value }).then(load, function (err) { no.disabled = false; add(out, msg('err', err.message)); });
            } }, 'Reject');
            add(card, h('div', { class: 'field' }, h('label', {}, 'Reason (for a rejection)'), note), yes, ' ', no, out);
          }
          add(holder, card);
        });
      }, function (err) { clear(holder); add(holder, msg('err', err.message)); });
    }
    load();
    return shell(h('div', {}, h('button', { class: 'ghost', onclick: function () { go('#/'); } }, '← All hotels'), h('h1', {}, 'Hotel applications'),
      h('p', { class: 'soft' }, 'Approving creates a hidden draft hotel and emails the owner an invitation to the BookDan Partner app. The hotel goes public only when you have set its map pin, rooms, prices, photos and business details and published it.'), holder));
  }

  // ---- routing
  function go(hash) { if (location.hash === hash) render(); else location.hash = hash; }
  function render() {
    clear(app);
    if (!state.token) { add(app, loginView()); return; }
    if (state.mustChange) { add(app, accountView(true)); return; }
    var m = location.hash.match(/^#\/hotel\/([0-9a-f-]{36})$/);
    if (m) add(app, hotelView(m[1], 'details'));
    else if (location.hash === '#/new' && can('hotels_write')) add(app, newView());
    else if (location.hash === '#/payouts' && can('finance')) add(app, payoutsView());
    else if (location.hash === '#/coupons' && state.role === 'admin') add(app, couponsView());
    else if (location.hash === '#/issues' && can('finance')) add(app, issuesView());
    else if (location.hash === '#/support' && can('support')) add(app, supportView());
    else if (location.hash === '#/rates' && state.role === 'admin') add(app, ratesView());
    else if (location.hash === '#/activity' && can('admins')) add(app, activityView());
    else if (location.hash === '#/calls' && state.role === 'admin') add(app, callsView());
    else if (location.hash === '#/reviews' && can('reviews')) add(app, reviewsView());
    else if (location.hash === '#/analytics' && state.role === 'admin') add(app, analyticsView());
    else if (location.hash === '#/applications' && can('applications')) add(app, applicationsView());
    else if (location.hash === '#/admins' && can('admins')) add(app, adminsView());
    else if (location.hash === '#/account' && state.role === 'admin') add(app, accountView(false));
    else if (/^#\/support\/[0-9a-f-]{36}$/.test(location.hash) && can('support')) add(app, ticketView(location.hash.slice(10)));
    else add(app, listView());
  }
  window.addEventListener('hashchange', render);

  call('GET', '/admin/config').then(function (cfg) {
    state.config = cfg;
    if (!state.token) return render();
    return loadMe().then(render, render);
  }, function (err) { clear(app); add(app, h('div', { class: 'card login' }, msg('err', err.message))); });
})();
