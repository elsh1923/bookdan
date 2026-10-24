// BookDan admin page. Plain JavaScript, no libraries. Every piece of text from the server is put on the page with textContent,
// never as HTML, so a hotel name can never run code.
(function () {
  'use strict';

  var API = '/v1';
  var AMENITIES = [['generator', 'Generator'], ['hot_water', 'Hot water'], ['wifi', 'Wi-Fi'], ['parking', 'Parking'], ['pay_at_hotel', 'Pay at hotel'], ['ac', 'Air conditioning'], ['pool', 'Swimming pool'], ['restaurant', 'Restaurant'], ['gym', 'Gym'], ['spa', 'Spa'], ['airport_shuttle', 'Airport shuttle'], ['laundry', 'Laundry'], ['front_desk_24h', '24-hour front desk']];
  var TYPES = [['hotel', 'Hotel'], ['guesthouse', 'Guesthouse'], ['lodge', 'Lodge'], ['resort', 'Resort']];
  var STATUSES = [['draft', 'Draft (not shown to guests)'], ['active', 'Published'], ['hidden', 'Hidden']];

  var state = { token: null, role: null, contact: '', config: { password: false, owners: false } };
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
    call('GET', '/admin/me').then(function (me) { state.contact = me.contact; render(); }, function () { render(); });
  }

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
      return h('form', { onsubmit: function (e) {
        e.preventDefault(); btn.disabled = true;
        call('POST', '/admin/login', { email: email.value, password: pass.value }).then(signedIn, function (err) { paint(err.message); });
      } },
        h('div', { class: 'field' }, h('label', {}, 'Email'), email),
        h('div', { class: 'field' }, h('label', {}, 'Password'), pass),
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
        h('span', { class: 'who' }, state.contact, ' ', h('span', { class: 'chip' }, state.role === 'admin' ? 'Platform admin' : 'Hotel owner'),
          h('button', { class: 'ghost', onclick: signOut }, 'Sign out'))),
      h('main', {}, content));
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
        state.role === 'admin' ? h('span', {}, h('button', { class: 'secondary', onclick: function () { go('#/coupons'); } }, 'Coupons'), ' ', h('button', { class: 'secondary', onclick: function () { go('#/payouts'); } }, 'Payouts'), ' ', h('button', { onclick: function () { go('#/new'); } }, 'Add a hotel')) : null),
      holder));
  }

  // ---- hotel form (shared by create and edit)
  function detailsForm(hotel, onSaved) {
    var isNew = !hotel;
    hotel = hotel || { name: '', type: 'hotel', city: '', area: '', address: '', phone: '', description: '', lat: '', lng: '', amenities: [], house_rules: [], pay_at_hotel: false, verified: false, status: 'draft', check_in_time: '14:00', check_out_time: '12:00' };
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
        check_in_time: f.checkIn.value, check_out_time: f.checkOut.value
      };
      if (isNaN(body.lat)) body.lat = null;
      if (isNaN(body.lng)) body.lng = null;
      if (state.role === 'admin') body.verified = f.verified.checked;
      var req = isNew ? call('POST', '/admin/hotels', body) : call('PUT', '/admin/hotels/' + hotel.id, body);
      req.then(function (res) { btn.disabled = false; if (isNew) go('#/hotel/' + res.id); else { add(out, msg('ok', 'Saved.')); onSaved && onSaved(); } },
        function (err) { btn.disabled = false; add(out, msg('err', err.message)); });
    } },
      h('div', { class: 'grid' },
        field('Name', f.name), field('Type', f.type), field('City', f.city), field('Area or neighbourhood', f.area),
        field('Street address', f.address), field('Phone for guests', f.phone),
        field('Latitude', f.lat), field('Longitude', f.lng, 'Tip: in Google Maps, right-click the hotel and click the numbers at the top to copy them.'),
        field('Check-in from', f.checkIn, 'Shown to guests on the hotel page and in their booking (Addis Ababa time).'), field('Check-out until', f.checkOut)),
      field('Description', f.description),
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

  // ---- routing
  function go(hash) { if (location.hash === hash) render(); else location.hash = hash; }
  function render() {
    clear(app);
    if (!state.token) { add(app, loginView()); return; }
    var m = location.hash.match(/^#\/hotel\/([0-9a-f-]{36})$/);
    if (m) add(app, hotelView(m[1], 'details'));
    else if (location.hash === '#/new' && state.role === 'admin') add(app, newView());
    else if (location.hash === '#/payouts' && state.role === 'admin') add(app, payoutsView());
    else if (location.hash === '#/coupons' && state.role === 'admin') add(app, couponsView());
    else add(app, listView());
  }
  window.addEventListener('hashchange', render);

  call('GET', '/admin/config').then(function (cfg) {
    state.config = cfg;
    if (!state.token) return render();
    return call('GET', '/admin/me').then(function (me) { state.contact = me.contact; state.role = me.role; render(); }, function () { render(); });
  }, function (err) { clear(app); add(app, h('div', { class: 'card login' }, msg('err', err.message))); });
})();
