package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
	"time"
)

func uploadTo(c *client, path, filename string, data []byte) resp {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("photo", filename)
	fw.Write(data)
	mw.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/v1"+path, &body)
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

func TestARoomHasItsOwnPhotosApartFromTheHotelGallery(t *testing.T) {
	admin := adminClient(t)
	h := admin.do("POST", "/admin/hotels", newHotelBody(fmt.Sprintf("Photo House %d", time.Now().UnixNano()%100000)))
	id := h.str("id")
	roomA := admin.do("POST", "/admin/hotels/"+id+"/rooms", map[string]any{"name": "Garden Room", "capacity": 2, "total_rooms": 3, "amenities": []string{}})
	roomB := admin.do("POST", "/admin/hotels/"+id+"/rooms", map[string]any{"name": "Roof Room", "capacity": 2, "total_rooms": 3, "amenities": []string{}})
	img := photoBytes(t, 800, 600)

	// A hotel photo and room photos land in different places.
	if r := upload(admin, id, "hotel.jpg", img); r.status != 200 {
		t.Fatalf("hotel photo: %d %s", r.status, r.raw)
	}
	var first, second string
	if r := uploadTo(admin, "/admin/hotels/"+id+"/rooms/"+roomA.str("id")+"/photos", "a1.jpg", img); r.status != 200 {
		t.Fatalf("room photo: %d %s", r.status, r.raw)
	} else {
		first = r.str("id")
	}
	if r := uploadTo(admin, "/admin/hotels/"+id+"/rooms/"+roomA.str("id")+"/photos", "a2.jpg", img); r.status != 200 {
		t.Fatalf("second room photo: %d %s", r.status, r.raw)
	} else {
		second = r.str("id")
	}
	d := admin.do("GET", "/admin/hotels/"+id, nil)
	if n := len(d.body["photos"].([]any)); n != 1 {
		t.Fatalf("the hotel gallery has only the hotel photo, not the room photos (got %d)", n)
	}
	roomPhotos := func(roomID string) []any {
		for _, r := range admin.do("GET", "/admin/hotels/"+id, nil).body["rooms"].([]any) {
			if m := r.(map[string]any); m["id"] == roomID {
				return m["photos"].([]any)
			}
		}
		return nil
	}
	if a, b := roomPhotos(roomA.str("id")), roomPhotos(roomB.str("id")); len(a) != 2 || len(b) != 0 {
		t.Fatalf("each room lists its own photos: A=%d B=%d", len(a), len(b))
	}
	if d := admin.do("GET", "/admin/hotels", nil); true {
		for _, it := range d.body["items"].([]any) {
			if m := it.(map[string]any); m["id"] == id && m["photos"].(float64) != 1 {
				t.Fatalf("the list counts only gallery photos: %v", m["photos"])
			}
		}
	}

	// Making the second room photo the cover moves only the room's photos.
	if r := admin.do("POST", "/admin/hotels/"+id+"/photos/"+second+"/cover", nil); r.status != 200 {
		t.Fatalf("cover: %d %s", r.status, r.raw)
	}
	if a := roomPhotos(roomA.str("id")); a[0].(map[string]any)["id"] != second || a[1].(map[string]any)["id"] != first {
		t.Fatalf("the chosen room photo should be first: %v", a)
	}
	if g := admin.do("GET", "/admin/hotels/"+id, nil).body["photos"].([]any); len(g) != 1 {
		t.Fatalf("the gallery is untouched")
	}

	// Another hotel's room, or a room that does not exist, is refused; a room holds at most ten.
	if r := uploadTo(admin, "/admin/hotels/"+id+"/rooms/"+standard+"/photos", "x.jpg", img); r.status != 404 {
		t.Fatalf("a room of another hotel: %d", r.status)
	}
	if r := uploadTo(admin, "/admin/hotels/"+id+"/rooms/not-a-room/photos", "x.jpg", img); r.status != 404 {
		t.Fatalf("an unknown room: %d", r.status)
	}
	for i := 2; i < 10; i++ {
		if r := uploadTo(admin, "/admin/hotels/"+id+"/rooms/"+roomA.str("id")+"/photos", "p.jpg", img); r.status != 200 {
			t.Fatalf("photo %d: %d %s", i+1, r.status, r.raw)
		}
	}
	if r := uploadTo(admin, "/admin/hotels/"+id+"/rooms/"+roomA.str("id")+"/photos", "p.jpg", img); r.status != 409 {
		t.Fatalf("the eleventh room photo: %d", r.status)
	}
	// The hotel's own gallery still has room (its limit is separate).
	if r := upload(admin, id, "hotel2.jpg", img); r.status != 200 {
		t.Fatalf("a second hotel photo: %d", r.status)
	}

	// Deleting a room photo works like any photo.
	if r := admin.do("DELETE", "/admin/hotels/"+id+"/photos/"+first, nil); r.status != 200 {
		t.Fatalf("delete: %d", r.status)
	}
	if a := roomPhotos(roomA.str("id")); len(a) != 9 {
		t.Fatalf("one photo fewer: %d", len(a))
	}
}

func TestGuestsSeeRoomPhotosOnTheRoomAndNotInTheHotelGallery(t *testing.T) {
	admin := adminClient(t)
	guest := newClient(t, "guest")
	img := photoBytes(t, 640, 480)
	r := uploadTo(admin, "/admin/hotels/"+meskel+"/rooms/"+family+"/photos", "family.jpg", img)
	if r.status != 200 {
		t.Fatalf("upload: %d %s", r.status, r.raw)
	}
	photo := r.str("id")
	defer admin.do("DELETE", "/admin/hotels/"+meskel+"/photos/"+photo, nil)

	d := guest.do("GET", fmt.Sprintf("/properties/%s?check_in=%s&check_out=%s", meskel, day(3), day(4)), nil)
	var found bool
	for _, rt := range d.body["room_types"].([]any) {
		m := rt.(map[string]any)
		photos := m["photos"].([]any)
		if m["id"] == family {
			found = len(photos) == 1 && m["photo_thumb"] != nil
		} else if len(photos) != 0 || m["photo_thumb"] != nil {
			t.Fatalf("a room without photos shows none: %v", m)
		}
	}
	if !found {
		t.Fatalf("the Family Suite should show its photo and a thumbnail: %s", d.raw)
	}
	for _, p := range d.body["property"].(map[string]any)["photos"].([]any) {
		if p.(string) == fmt.Sprintf("http://example.test/v1/photos/%s/main", photo) {
			t.Fatalf("a room photo must not appear in the hotel gallery")
		}
	}
	// The photo itself is public.
	res, err := http.Get(srv.URL + "/v1/photos/" + photo + "/thumb")
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("the thumbnail should be served: %v %v", err, res)
	}
	res.Body.Close()
}
