package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"
)

const adminPassword = "correct-horse-battery-staple"

func adminClient(t *testing.T) *client {
	t.Helper()
	c := newClient(t, "admin")
	r := c.do("POST", "/admin/login", map[string]string{"email": "admin@bookdan.test", "password": adminPassword})
	if r.status != 200 || r.str("role") != "admin" {
		t.Fatalf("admin login: %d %s", r.status, r.raw)
	}
	c.token = r.str("token")
	return c
}

func photoBytes(t *testing.T, w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

func upload(c *client, hotelID, filename string, data []byte) resp {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("photo", filename)
	fw.Write(data)
	mw.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/v1/admin/hotels/"+hotelID+"/photos", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{status: res.StatusCode, raw: raw}
	json.Unmarshal(raw, &out.body)
	return out
}

func newHotelBody(name string) map[string]any {
	return map[string]any{
		"name": name, "type": "guesthouse", "city": "Addis Ababa", "area": "Kazanchis", "address": "Kazanchis, Addis Ababa", "phone": "+251 11 555 0100",
		"description": "A new place.", "lat": 9.017, "lng": 38.77, "amenities": []string{"wifi", "hot_water"}, "house_rules": []string{"No smoking", ""}, "pay_at_hotel": true,
	}
}

func TestTheAdminPageIsServedWithAStrictPolicy(t *testing.T) {
	res, err := http.Get(srv.URL + "/admin")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.Contains(string(body), "BookDan Admin") {
		t.Fatalf("admin page: %d", res.StatusCode)
	}
	csp := res.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "unsafe-inline") || res.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatalf("the page must be locked down: csp=%q", csp)
	}
	for _, p := range []string{"/admin/app.js", "/admin/style.css"} {
		r, _ := http.Get(srv.URL + p)
		r.Body.Close()
		if r.StatusCode != 200 {
			t.Fatalf("%s: %d", p, r.StatusCode)
		}
	}
}

func TestAdminSignInIsStrict(t *testing.T) {
	c := newClient(t, "admin")
	if r := c.do("POST", "/admin/login", map[string]string{"email": "admin@bookdan.test", "password": "wrong"}); r.status != 401 {
		t.Fatalf("wrong password: %d", r.status)
	}
	if r := c.do("POST", "/admin/login", map[string]string{"email": "someone@else.com", "password": adminPassword}); r.status != 401 {
		t.Fatalf("an email that is not on the admin list must not get in even with the right password: %d", r.status)
	}
	if r := c.do("GET", "/admin/hotels", nil); r.status != 401 {
		t.Fatalf("no token: %d", r.status)
	}
	// Guest and hotel tokens are not admin tokens.
	g := newClient(t, "guest")
	g.signIn("email", uniqueEmail("ga"))
	if r := g.do("GET", "/admin/hotels", nil); r.status != 401 {
		t.Fatalf("a guest token on the admin API: %d", r.status)
	}
	// Guessing is slowed down: the ninth try within ten minutes is refused whatever the password.
	var last int
	for i := 0; i < 10; i++ {
		last = c.do("POST", "/admin/login", map[string]string{"email": "guesser@x.com", "password": fmt.Sprint(i)}).status
	}
	if last != 429 {
		t.Fatalf("password guessing must be rate limited, last status %d", last)
	}
}

func TestAdminBuildsAHotelEndToEnd(t *testing.T) {
	admin := adminClient(t)
	name := fmt.Sprintf("Kazanchis Test House %d", time.Now().UnixNano()%100000)

	bad := newHotelBody(name)
	bad["lat"] = 120.0
	if r := admin.do("POST", "/admin/hotels", bad); r.status != 400 {
		t.Fatalf("an impossible latitude: %d", r.status)
	}
	bad = newHotelBody(name)
	bad["amenities"] = []string{"jacuzzi"}
	if r := admin.do("POST", "/admin/hotels", bad); r.status != 400 {
		t.Fatalf("an unknown feature: %d", r.status)
	}
	created := admin.do("POST", "/admin/hotels", newHotelBody(name))
	if created.status != 200 {
		t.Fatalf("create: %d %s", created.status, created.raw)
	}
	id := created.str("id")

	search := func() string {
		return string(admin.do("GET", fmt.Sprintf("/search?q=%s&check_in=%s&check_out=%s&guests=1", strings.ReplaceAll(name, " ", "%20"), day(200), day(201)), nil).raw)
	}
	if strings.Contains(search(), name) {
		t.Fatalf("a draft hotel must not be shown to guests")
	}
	// It cannot be published before it has a room with a price.
	body := newHotelBody(name)
	body["status"] = "active"
	if r := admin.do("PUT", "/admin/hotels/"+id, body); r.status != 409 {
		t.Fatalf("publishing an empty hotel: %d %s", r.status, r.raw)
	}

	// A room, then a price (the price must be a whole number of birr).
	room := admin.do("POST", "/admin/hotels/"+id+"/rooms", map[string]any{"name": "Standard", "capacity": 2, "total_rooms": 3, "amenities": []string{"TV", " "}})
	if room.status != 200 {
		t.Fatalf("room: %d %s", room.status, room.raw)
	}
	if r := admin.do("POST", "/admin/hotels/"+id+"/rooms", map[string]any{"name": "Standard", "capacity": 2, "total_rooms": 1}); r.status != 409 {
		t.Fatalf("a duplicate room name: %d", r.status)
	}
	rid := room.str("id")
	if r := admin.do("PUT", "/admin/hotels/"+id+"/rooms/"+rid+"/price", map[string]any{"price_santim": 150}); r.status != 400 {
		t.Fatalf("a price of 1.50 birr must be refused: %d", r.status)
	}
	if r := admin.do("PUT", "/admin/hotels/"+id+"/rooms/"+rid+"/price", map[string]any{"price_santim": 250000}); r.status != 200 {
		t.Fatalf("price: %d %s", r.status, r.raw)
	}

	// Photos: a bad file is refused; two good ones are stored, the first is the cover.
	if r := upload(admin, id, "notes.txt", []byte("not a picture")); r.status != 400 {
		t.Fatalf("a text file as a photo: %d", r.status)
	}
	p1 := upload(admin, id, "front.jpg", photoBytes(t, 2400, 1800))
	p2 := upload(admin, id, "room.jpg", photoBytes(t, 800, 600))
	if p1.status != 200 || p2.status != 200 {
		t.Fatalf("uploads: %d %d %s", p1.status, p2.status, p1.raw)
	}

	// Now it can be published, and guests see it with its cover photo.
	if r := admin.do("PUT", "/admin/hotels/"+id, body); r.status != 200 {
		t.Fatalf("publish: %d %s", r.status, r.raw)
	}
	if !strings.Contains(search(), name) {
		t.Fatalf("a published hotel must appear in search")
	}
	g := newClient(t, "guest")
	list := g.do("GET", fmt.Sprintf("/search?q=%s&check_in=%s&check_out=%s&guests=1", strings.ReplaceAll(name, " ", "%20"), day(200), day(201)), nil)
	item := list.body["items"].([]any)[0].(map[string]any)
	coverURL, _ := item["photo_url"].(string)
	if !strings.HasSuffix(coverURL, "/thumb") || !strings.Contains(coverURL, p1.str("id")) {
		t.Fatalf("search should show the first photo's thumbnail, got %q", coverURL)
	}
	detail := g.do("GET", fmt.Sprintf("/properties/%s?check_in=%s&check_out=%s", id, day(200), day(201)), nil)
	photos, _ := detail.body["property"].(map[string]any)["photos"].([]any)
	if len(photos) != 2 || !strings.HasSuffix(photos[0].(string), "/main") {
		t.Fatalf("the hotel page should list both photos at full size: %v", photos)
	}

	// Photos are public and cached for a year.
	res, err := http.Get(strings.Replace(coverURL, "http://example.test", srv.URL, 1))
	if err != nil {
		t.Fatal(err)
	}
	img, _ := io.ReadAll(res.Body)
	res.Body.Close()
	cfg, _, derr := image.DecodeConfig(bytes.NewReader(img))
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/jpeg" || !strings.Contains(res.Header.Get("Cache-Control"), "immutable") || derr != nil || cfg.Width != 640 {
		t.Fatalf("thumbnail: status %d type %s width %d", res.StatusCode, res.Header.Get("Content-Type"), cfg.Width)
	}

	// Making the second photo the cover puts it first.
	if r := admin.do("POST", "/admin/hotels/"+id+"/photos/"+p2.str("id")+"/cover", nil); r.status != 200 {
		t.Fatalf("cover: %d", r.status)
	}
	d := admin.do("GET", "/admin/hotels/"+id, nil)
	first := d.body["photos"].([]any)[0].(map[string]any)
	if !strings.Contains(first["thumb"].(string), p2.str("id")) || first["cover"] != true {
		t.Fatalf("the chosen photo should now be the cover: %v", first)
	}
	if r := admin.do("DELETE", "/admin/hotels/"+id+"/photos/"+p2.str("id"), nil); r.status != 200 {
		t.Fatalf("delete photo: %d", r.status)
	}

	// The new hotel is bookable end to end (inventory is created on demand from the room count).
	g.signIn("email", uniqueEmail("nh"))
	withProfile(g)
	q := quote(g, id, rid, 200, 201, 2, 1)
	if q.status != 200 || q.num("total_santim") != 2500*100*115/100 {
		t.Fatalf("quote for the new hotel: %d %s", q.status, q.raw)
	}

	// An admin can see who changed what: every change is in the audit log.
	var n int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE details->>'property_id' = $1 AND action LIKE 'admin_%'`, id).Scan(&n)
	if n < 8 {
		t.Fatalf("admin changes must be audited, found %d entries", n)
	}
}

func TestShrinkingRoomsBelowWhatIsBookedIsRefused(t *testing.T) {
	admin := adminClient(t)
	g := newClient(t, "guest")
	g.signIn("email", uniqueEmail("shr"))
	withProfile(g)
	q := quote(g, meskel, family, 250, 251, 2, 1)
	hold(g, q.str("id"), "key-shrink-1")
	defer func() { // other tests expect the one Family Suite the demo data has
		admin.do("PUT", "/admin/hotels/"+meskel+"/rooms/"+suiteID(admin), map[string]any{"name": "Family Suite", "capacity": 4, "total_rooms": 1})
	}()
	d := admin.do("GET", "/admin/hotels/"+meskel, nil)
	var rid string
	var cap float64
	for _, r := range d.body["rooms"].([]any) {
		if r.(map[string]any)["name"] == "Family Suite" {
			rid, cap = r.(map[string]any)["id"].(string), r.(map[string]any)["capacity"].(float64)
		}
	}
	r := admin.do("PUT", "/admin/hotels/"+meskel+"/rooms/"+rid, map[string]any{"name": "Family Suite", "capacity": int(cap), "total_rooms": 0})
	if r.status != 409 {
		t.Fatalf("going to 0 rooms while one is held must be refused: %d %s", r.status, r.raw)
	}
	if r := admin.do("PUT", "/admin/hotels/"+meskel+"/rooms/"+rid, map[string]any{"name": "Family Suite", "capacity": int(cap), "total_rooms": 3}); r.status != 200 {
		t.Fatalf("growing is fine: %d %s", r.status, r.raw)
	}
	// Growing really adds rooms for the coming nights: 3 suites, 1 held, so 2 can be sold and 3 cannot.
	if q2 := quote(g, meskel, family, 250, 251, 2, 2); q2.status != 200 {
		t.Fatalf("2 suites should now be sellable: %d %s", q2.status, q2.raw)
	}
	if q3 := quote(g, meskel, family, 250, 251, 3, 3); q3.str("error", "code") != "SOLD_OUT" {
		t.Fatalf("3 suites cannot be sold while one is held: %s", q3.raw)
	}
}

func TestOwnersManageOnlyTheirOwnHotelAndCannotCreateOrStaff(t *testing.T) {
	admin := adminClient(t)
	h1 := admin.do("POST", "/admin/hotels", newHotelBody(fmt.Sprintf("Owner One %d", time.Now().UnixNano()%100000)))
	h2 := admin.do("POST", "/admin/hotels", newHotelBody(fmt.Sprintf("Owner Two %d", time.Now().UnixNano()%100000)))
	ownerEmail, recEmail := uniqueEmail("owner"), uniqueEmail("rec")

	if r := admin.do("POST", "/admin/hotels/"+h1.str("id")+"/staff", map[string]string{"contact": ownerEmail, "role": "owner"}); r.status != 200 {
		t.Fatalf("add owner: %d %s", r.status, r.raw)
	}
	if r := admin.do("POST", "/admin/hotels/"+h1.str("id")+"/staff", map[string]string{"contact": recEmail, "role": "receptionist"}); r.status != 200 {
		t.Fatalf("add receptionist: %d %s", r.status, r.raw)
	}
	if r := admin.do("POST", "/admin/hotels/"+h1.str("id")+"/staff", map[string]string{"contact": "x", "role": "owner"}); r.status != 400 {
		t.Fatalf("a bad contact: %d", r.status)
	}

	// The owner signs in with a code (the admin page's owner tab).
	owner := newClient(t, "admin")
	owner.do("POST", "/admin/otp/request", map[string]string{"type": "email", "value": ownerEmail})
	v := owner.do("POST", "/admin/otp/verify", map[string]string{"type": "email", "value": ownerEmail, "code": "123456"})
	if v.status != 200 || v.str("role") != "owner" {
		t.Fatalf("owner sign-in: %d %s", v.status, v.raw)
	}
	owner.token = v.str("token")

	list := owner.do("GET", "/admin/hotels", nil)
	items := list.body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != h1.str("id") {
		t.Fatalf("the owner should see only their own hotel: %s", list.raw)
	}
	if r := owner.do("GET", "/admin/hotels/"+h2.str("id"), nil); r.status != 404 {
		t.Fatalf("another hotel must look like it does not exist: %d", r.status)
	}
	if r := owner.do("PUT", "/admin/hotels/"+h2.str("id"), newHotelBody("Hijacked")); r.status != 404 {
		t.Fatalf("editing another hotel: %d", r.status)
	}
	if r := upload(owner, h2.str("id"), "x.jpg", photoBytes(t, 100, 100)); r.status != 404 {
		t.Fatalf("uploading to another hotel: %d", r.status)
	}
	if r := owner.do("POST", "/admin/hotels", newHotelBody("Brand new")); r.status != 403 {
		t.Fatalf("owners cannot create hotels: %d", r.status)
	}
	if r := owner.do("POST", "/admin/hotels/"+h1.str("id")+"/staff", map[string]string{"contact": uniqueEmail("x"), "role": "owner"}); r.status != 403 {
		t.Fatalf("owners cannot add staff: %d", r.status)
	}
	// An owner edits their own hotel but cannot give themselves the Verified badge.
	body := newHotelBody("Owner One Renamed")
	body["verified"] = true
	if r := owner.do("PUT", "/admin/hotels/"+h1.str("id"), body); r.status != 200 {
		t.Fatalf("owner edits own hotel: %d %s", r.status, r.raw)
	}
	if d := owner.do("GET", "/admin/hotels/"+h1.str("id"), nil); d.body["verified"] != false || d.body["name"] != "Owner One Renamed" {
		t.Fatalf("name changes, verified does not: %v %v", d.body["name"], d.body["verified"])
	}

	// A receptionist cannot use the admin page at all.
	rec := newClient(t, "admin")
	rec.do("POST", "/admin/otp/request", map[string]string{"type": "email", "value": recEmail})
	if r := rec.do("POST", "/admin/otp/verify", map[string]string{"type": "email", "value": recEmail, "code": "123456"}); r.status != 403 {
		t.Fatalf("a receptionist on the admin page: %d %s", r.status, r.raw)
	}
	// A stranger cannot either.
	st := newClient(t, "admin")
	stranger := uniqueEmail("stranger")
	st.do("POST", "/admin/otp/request", map[string]string{"type": "email", "value": stranger})
	if r := st.do("POST", "/admin/otp/verify", map[string]string{"type": "email", "value": stranger, "code": "123456"}); r.status != 403 {
		t.Fatalf("a stranger on the admin page: %d", r.status)
	}

	// Staff can be removed by the admin, and the hotel app starts to see the hotel.
	hotelApp := newClient(t, "partner")
	hotelApp.signIn("email", ownerEmail)
	if hb := hotelApp.do("GET", "/partner/bookings", nil); hb.status != 200 {
		t.Fatalf("the new owner should also work in the hotel app: %d", hb.status)
	}
	d := admin.do("GET", "/admin/hotels/"+h1.str("id"), nil)
	for _, s := range d.body["staff"].([]any) {
		sm := s.(map[string]any)
		if sm["contact"] == recEmail {
			if r := admin.do("DELETE", "/admin/hotels/"+h1.str("id")+"/staff/"+sm["user_id"].(string), nil); r.status != 200 {
				t.Fatalf("remove staff: %d", r.status)
			}
		}
	}
	if r := admin.do("DELETE", "/admin/hotels/"+h1.str("id")+"/staff/"+"00000000-0000-0000-0000-000000000000", nil); r.status != 404 {
		t.Fatalf("removing someone who is not staff: %d", r.status)
	}
}

func suiteID(admin *client) string {
	d := admin.do("GET", "/admin/hotels/"+meskel, nil)
	for _, r := range d.body["rooms"].([]any) {
		if r.(map[string]any)["name"] == "Family Suite" {
			return r.(map[string]any)["id"].(string)
		}
	}
	return ""
}
