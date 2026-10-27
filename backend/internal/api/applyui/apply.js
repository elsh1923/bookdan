(function () {
  'use strict';
  var form = document.getElementById('form');
  var error = document.getElementById('error');
  var send = document.getElementById('send');

  function show(msg) { error.textContent = msg; error.hidden = !msg; if (msg) error.scrollIntoView({ block: 'center' }); }

  form.addEventListener('submit', function (e) {
    e.preventDefault();
    show('');
    var d = new FormData(form);
    var body = {};
    ['hotel_name', 'hotel_type', 'city', 'area', 'address', 'hotel_phone', 'description', 'owner_name', 'owner_email', 'owner_phone', 'legal_name', 'license_number', 'website'].forEach(function (k) { body[k] = String(d.get(k) || ''); });
    body.rooms_count = parseInt(d.get('rooms_count'), 10) || 0;
    body.pay_at_hotel = form.elements.pay_at_hotel.checked;
    send.disabled = true;
    fetch('/v1/hotel-applications', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }).then(function (res) {
      return res.json().catch(function () { return {}; }).then(function (json) {
        if (!res.ok) throw new Error((json.error && json.error.message) || 'Something went wrong. Please try again.');
        document.getElementById('ref').textContent = json.reference;
        form.hidden = true;
        document.getElementById('done').hidden = false;
        window.scrollTo(0, 0);
      });
    }).catch(function (err) {
      send.disabled = false;
      show(err.message === 'Failed to fetch' ? 'No connection. Check your internet and try again.' : err.message);
    });
  });
})();
