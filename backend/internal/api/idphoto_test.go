package api_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
	"time"
)

// pic makes a picture that looks different for each shade, as a PNG (the server must turn every upload into a JPEG).
func pic(t *testing.T, shade uint8) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 320, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 320; x++ {
			img.Set(x, y, color.RGBA{shade, uint8(x), uint8(y), 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestIDPhotosAreEncryptedFrozenWithTheBookingAndErasedWithTheNumber(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("idphoto"))
	withProfile(guest)
	hotel := hotelFor(t)

	// Only a real picture, only front or back.
	if r := guest.do("PUT", "/me/id-photo/front", map[string]string{"image": base64.StdEncoding.EncodeToString([]byte("not a picture"))}); r.status != 400 {
		t.Fatalf("a file that is not a picture: %d %s", r.status, r.raw)
	}
	if r := guest.do("PUT", "/me/id-photo/side", map[string]string{"image": pic(t, 10)}); r.status != 400 {
		t.Fatalf("an unknown side: %d", r.status)
	}
	if r := guest.do("PUT", "/me/id-photo/front", map[string]string{"image": pic(t, 10)}); r.status != 200 {
		t.Fatalf("front: %d %s", r.status, r.raw)
	}
	if r := guest.do("PUT", "/me/id-photo/back", map[string]string{"image": "data:image/png;base64," + pic(t, 20)}); r.status != 200 {
		t.Fatalf("back (a data URL is accepted): %d %s", r.status, r.raw)
	}
	prof := guest.do("GET", "/me/profile", nil)
	photos, _ := prof.body["profile"].(map[string]any)["id_photos"].(map[string]any)
	if photos["front"] != true || photos["back"] != true {
		t.Fatalf("the profile says which sides exist: %s", prof.raw)
	}

	// The guest gets back a plain JPEG; the database holds only ciphertext.
	own := guest.do("GET", "/me/id-photo/front", nil)
	if own.status != 200 {
		t.Fatalf("own photo: %d", own.status)
	}
	if _, err := jpeg.Decode(bytes.NewReader(own.raw)); err != nil {
		t.Fatalf("the photo is returned as a JPEG: %v", err)
	}
	var enc []byte
	pool.QueryRow(ctx, `SELECT g.data_enc FROM guest_id_photos g JOIN users u ON u.id = g.user_id JOIN guest_profiles p ON p.user_id = u.id WHERE p.full_name = 'Hanna Tesfaye' AND g.side = 'front' ORDER BY g.updated_at DESC LIMIT 1`).Scan(&enc)
	if len(enc) == 0 || bytes.Contains(enc, []byte{0xFF, 0xD8, 0xFF}) {
		t.Fatalf("the stored photo must be encrypted, not a JPEG")
	}
	if other := newClient(t, "guest"); true {
		other.signIn("email", uniqueEmail("idother"))
		if r := other.do("GET", "/me/id-photo/front", nil); r.status != 404 {
			t.Fatalf("another guest has no photo and cannot see mine: %d", r.status)
		}
	}

	// A booking takes a frozen copy. The hotel cannot see it while the booking is only held.
	b := hold(guest, quote(guest, meskel, standard, 260, 262, 1, 1).str("id"), fmt.Sprintf("idphoto-%d", n))
	id := b.str("id")
	if r := hotel.do("GET", "/partner/bookings/"+id+"/guest/id-photo/front", nil); r.status != 404 {
		t.Fatalf("a held booking is invisible to the hotel: %d", r.status)
	}
	payOnline(t, guest, id, "succeeded")
	seen := hotel.do("GET", "/partner/bookings/"+id+"/guest/id-photo/front", nil)
	if seen.status != 200 || !bytes.Equal(seen.raw, own.raw) {
		t.Fatalf("the hotel sees the guest's photo: %d", seen.status)
	}
	if list := hotel.do("GET", "/partner/bookings", nil); !bytes.Contains(list.raw, []byte(`"id_photos":{"front":true,"back":true}`)) {
		t.Fatalf("the hotel's booking says which photo sides exist: %s", list.raw)
	}
	var audited int
	pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = 'guest_id_photo_viewed' AND details->>'booking_id' = $1`, id).Scan(&audited)
	if audited != 1 {
		t.Fatalf("every view is written to the audit log: %d", audited)
	}

	// Changing or deleting the guest's own photo never changes what the hotel holds.
	guest.do("PUT", "/me/id-photo/front", map[string]string{"image": pic(t, 200)})
	if again := hotel.do("GET", "/partner/bookings/"+id+"/guest/id-photo/front", nil); !bytes.Equal(again.raw, seen.raw) {
		t.Fatalf("the hotel's copy is frozen")
	}
	if r := guest.do("DELETE", "/me/id-photo/back", nil); r.status != 200 {
		t.Fatalf("delete: %d", r.status)
	}
	if r := guest.do("GET", "/me/id-photo/back", nil); r.status != 404 {
		t.Fatalf("a deleted photo is gone: %d", r.status)
	}
	if r := hotel.do("GET", "/partner/bookings/"+id+"/guest/id-photo/back", nil); r.status != 200 {
		t.Fatalf("the hotel's copy outlives the guest's deletion: %d", r.status)
	}

	// After the retention period the hotel's photos are erased with the ID number.
	pool.Exec(ctx, `UPDATE bookings SET check_in = `+addisToday+` - 47, check_out = `+addisToday+` - 45 WHERE id = $1`, id)
	app.RunMaintenance(ctx)
	if r := hotel.do("GET", "/partner/bookings/"+id+"/guest/id-photo/front", nil); r.status != 410 {
		t.Fatalf("an erased photo is gone: %d %s", r.status, r.raw)
	}
	var left int
	pool.QueryRow(ctx, `SELECT count(*) FROM booking_id_photos WHERE booking_id = $1 AND data_enc IS NOT NULL`, id).Scan(&left)
	if left != 0 {
		t.Fatalf("no ciphertext may remain after the erasure")
	}

	// Deleting the profile deletes the guest's photos too.
	if r := guest.do("DELETE", "/me/profile", nil); r.status != 200 {
		t.Fatalf("delete profile: %d", r.status)
	}
	if r := guest.do("GET", "/me/id-photo/front", nil); r.status != 404 {
		t.Fatalf("photos go with the profile: %d", r.status)
	}
}
