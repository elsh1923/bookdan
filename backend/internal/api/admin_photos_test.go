package api

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// testJPEG makes a w x h picture, optionally with an EXIF orientation (what phones write when held sideways).
func testJPEG(t *testing.T, w, h, orientation int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 255 / w), uint8(y * 255 / h), 120, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	if orientation == 0 {
		return data
	}
	// A minimal EXIF block (big-endian TIFF with one tag: orientation) right after the JPEG start marker.
	tiff := []byte{'M', 'M', 0, 42, 0, 0, 0, 8, 0, 1, 0x01, 0x12, 0, 3, 0, 0, 0, 1, 0, byte(orientation), 0, 0, 0, 0, 0, 0}
	body := append([]byte("Exif\x00\x00"), tiff...)
	size := len(body) + 2
	seg := append([]byte{0xFF, 0xE1, byte(size >> 8), byte(size)}, body...)
	return append(append([]byte{0xFF, 0xD8}, seg...), data[2:]...)
}

func TestPhotosAreShrunkAndTurnedUpright(t *testing.T) {
	// A big landscape camera photo becomes a 1600 px picture and a 640 px thumbnail.
	main, thumb, w, h, err := processPhoto(testJPEG(t, 4000, 3000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if w != 1600 || h != 1200 {
		t.Fatalf("main size = %dx%d, want 1600x1200", w, h)
	}
	tc, _, _ := image.DecodeConfig(bytes.NewReader(thumb))
	if tc.Width != 640 || tc.Height != 480 {
		t.Fatalf("thumbnail = %dx%d, want 640x480", tc.Width, tc.Height)
	}
	if len(thumb) >= len(main) {
		t.Fatalf("the thumbnail (%d bytes) should be smaller than the main picture (%d bytes)", len(thumb), len(main))
	}

	// A small picture is never scaled up.
	_, _, w, h, _ = processPhoto(testJPEG(t, 300, 200, 0))
	if w != 300 || h != 200 {
		t.Fatalf("a small picture must stay %dx%d, got %dx%d", 300, 200, w, h)
	}

	// A phone held upright writes a landscape picture with orientation 6 (rotate 90 degrees): it must come out portrait.
	_, _, w, h, err = processPhoto(testJPEG(t, 400, 200, 6))
	if err != nil {
		t.Fatal(err)
	}
	if w != 200 || h != 400 {
		t.Fatalf("orientation 6 should turn 400x200 into 200x400, got %dx%d", w, h)
	}
	for o := 1; o <= 8; o++ {
		if _, _, _, _, err := processPhoto(testJPEG(t, 60, 40, o)); err != nil {
			t.Fatalf("orientation %d: %v", o, err)
		}
	}

	// PNG with transparency is accepted and flattened.
	var pngBuf bytes.Buffer
	p := image.NewNRGBA(image.Rect(0, 0, 50, 50))
	png.Encode(&pngBuf, p)
	if _, _, _, _, err := processPhoto(pngBuf.Bytes()); err != nil {
		t.Fatalf("png: %v", err)
	}
}

func TestPhotosThatAreNotPicturesAreRefused(t *testing.T) {
	for name, data := range map[string][]byte{
		"text":       []byte("hello, I am not a picture"),
		"empty":      {},
		"html":       []byte("<html><script>alert(1)</script></html>"),
		"truncated":  testJPEG(t, 100, 100, 0)[:40],
		"fake jpeg":  append([]byte{0xFF, 0xD8, 0xFF}, bytes.Repeat([]byte{0}, 100)...),
		"pdf header": []byte("%PDF-1.4 ..."),
	} {
		if _, _, _, _, err := processPhoto(data); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
}
