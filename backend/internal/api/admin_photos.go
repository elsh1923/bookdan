package api

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png" // decoders: JPEG (above), PNG and WebP uploads
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/bookdan/backend/internal/httpx"
)

const (
	maxUploadBytes    = 10 << 20 // one photo, as uploaded
	maxPixels         = 40_000_000
	mainSide          = 1600 // longest side of the full-size copy
	thumbSide         = 640  // longest side of the small copy used in lists
	maxPhotosPerHotel = 15
)

var errNotAnImage = errors.New("not an image")

// processPhoto turns an uploaded picture into two JPEGs: the full size (at most 1600 px) and a small one (at most 640 px), so a
// guest on mobile data downloads a few hundred kilobytes, not a camera original. It applies the camera's rotation, flattens
// transparency onto white and refuses anything that is not a real, reasonably sized JPEG, PNG or WebP.
func processPhoto(data []byte) (main, thumb []byte, w, h int, err error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 {
		return nil, nil, 0, 0, errNotAnImage
	}
	if cfg.Width*cfg.Height > maxPixels {
		return nil, nil, 0, 0, errors.New("the picture has too many pixels")
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, nil, 0, 0, errNotAnImage
	}
	src = applyOrientation(src, jpegOrientation(data))

	// Flatten onto white (PNG and WebP may have transparent parts).
	b := src.Bounds()
	flat := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(flat, flat.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), src, b.Min, draw.Over)

	full := fit(flat, mainSide)
	small := fit(flat, thumbSide)
	if main, err = encodeJPEG(full, 82); err != nil {
		return nil, nil, 0, 0, err
	}
	if thumb, err = encodeJPEG(small, 74); err != nil {
		return nil, nil, 0, 0, err
	}
	return main, thumb, full.Bounds().Dx(), full.Bounds().Dy(), nil
}

func encodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// fit scales the image down so its longest side is at most side. It never scales up.
func fit(src image.Image, side int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= side && h <= side {
		return src
	}
	nw, nh := side, h*side/w
	if h > w {
		nw, nh = w*side/h, side
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Src, nil)
	return dst
}

// jpegOrientation reads the EXIF "orientation" of a JPEG (1 to 8). 1 means upright and is also the answer when there is none.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		if marker == 0xDA || marker == 0xD9 { // start of picture data: no EXIF before this
			return 1
		}
		size := int(data[i+2])<<8 | int(data[i+3])
		if size < 2 || i+2+size > len(data) {
			return 1
		}
		if marker == 0xE1 && size >= 16 && string(data[i+4:i+10]) == "Exif\x00\x00" {
			return tiffOrientation(data[i+10 : i+2+size])
		}
		i += 2 + size
	}
	return 1
}

func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var u16 func([]byte) int
	var u32 func([]byte) int
	switch string(t[:2]) {
	case "II":
		u16 = func(b []byte) int { return int(b[0]) | int(b[1])<<8 }
		u32 = func(b []byte) int { return int(b[0]) | int(b[1])<<8 | int(b[2])<<16 | int(b[3])<<24 }
	case "MM":
		u16 = func(b []byte) int { return int(b[0])<<8 | int(b[1]) }
		u32 = func(b []byte) int { return int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3]) }
	default:
		return 1
	}
	off := u32(t[4:8])
	if off < 8 || off+2 > len(t) {
		return 1
	}
	n := u16(t[off : off+2])
	for k := 0; k < n; k++ {
		e := off + 2 + k*12
		if e+12 > len(t) {
			return 1
		}
		if u16(t[e:e+2]) == 0x0112 { // orientation tag
			if v := u16(t[e+8 : e+10]); v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// applyOrientation turns a picture upright according to its EXIF orientation.
func applyOrientation(src image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	swap := o >= 5
	nw, nh := w, h
	if swap {
		nw, nh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

// ---- endpoints

func (s *Server) adminUploadPhoto(w http.ResponseWriter, r *http.Request) error {
	id := chi.URLParam(r, "id")
	if err := s.canManage(r, id); err != nil {
		return err
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		return fail(http.StatusRequestEntityTooLarge, httpx.CodeInvalidRequest, "The photo is too big (the limit is 10 MB)")
	}
	file, _, err := r.FormFile("photo")
	if err != nil {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Choose a photo to upload")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
	if err != nil || len(data) > maxUploadBytes {
		return fail(http.StatusRequestEntityTooLarge, httpx.CodeInvalidRequest, "The photo is too big (the limit is 10 MB)")
	}
	main, thumb, pw, ph, err := processPhoto(data)
	if err != nil {
		if errors.Is(err, errNotAnImage) {
			return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "That file is not a photo. Use JPEG, PNG or WebP.")
		}
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "That picture cannot be used: "+err.Error())
	}
	var photoID string
	err = s.tx(r.Context(), func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM property_photos WHERE property_id = $1`, id).Scan(&count); err != nil {
			return err
		}
		if count >= maxPhotosPerHotel {
			return fail(http.StatusConflict, codeInvalidState, "A hotel can have at most 15 photos. Delete one first.")
		}
		if err := tx.QueryRow(r.Context(), `INSERT INTO property_photos (property_id, position, main, thumb, width, height, uploaded_by)
			VALUES ($1, coalesce((SELECT max(position) + 1 FROM property_photos WHERE property_id = $1), 0), $2, $3, $4, $5, $6) RETURNING id`,
			id, main, thumb, pw, ph, claimsFrom(r).Sub).Scan(&photoID); err != nil {
			return err
		}
		return s.adminAudit(r, tx, "admin_photo_added", id, map[string]any{"photo_id": photoID})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		ID    string `json:"id"`
		Main  string `json:"main"`
		Thumb string `json:"thumb"`
	}{httpx.NowMeta(), photoID, s.photoURL(photoID, "main"), s.photoURL(photoID, "thumb")})
	return nil
}

func (s *Server) adminCoverPhoto(w http.ResponseWriter, r *http.Request) error {
	id, photo := chi.URLParam(r, "id"), chi.URLParam(r, "photo")
	if err := s.canManage(r, id); err != nil {
		return err
	}
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM property_photos WHERE id = $1 AND property_id = $2)`, photo, id).Scan(&exists); err != nil {
			if isBadUUID(err) {
				return errNotFound
			}
			return err
		}
		if !exists {
			return errNotFound
		}
		// Everyone else moves one place down; the chosen photo takes place 0.
		if _, err := tx.Exec(r.Context(), `UPDATE property_photos SET position = position + 1 WHERE property_id = $1 AND id <> $2`, id, photo); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE property_photos SET position = 0 WHERE id = $1`, photo); err != nil {
			return err
		}
		return s.adminAudit(r, tx, "admin_photo_cover", id, map[string]any{"photo_id": photo})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

func (s *Server) adminDeletePhoto(w http.ResponseWriter, r *http.Request) error {
	id, photo := chi.URLParam(r, "id"), chi.URLParam(r, "photo")
	if err := s.canManage(r, id); err != nil {
		return err
	}
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		tag, err := tx.Exec(r.Context(), `DELETE FROM property_photos WHERE id = $1 AND property_id = $2`, photo, id)
		if err != nil {
			if isBadUUID(err) {
				return errNotFound
			}
			return err
		}
		if tag.RowsAffected() == 0 {
			return errNotFound
		}
		return s.adminAudit(r, tx, "admin_photo_deleted", id, map[string]any{"photo_id": photo})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

// servePhoto is public: photos are advertising. A photo never changes once stored, so browsers and phones may keep it forever.
func (s *Server) servePhoto(w http.ResponseWriter, r *http.Request) error {
	col := ""
	switch chi.URLParam(r, "size") {
	case "main":
		col = "main"
	case "thumb":
		col = "thumb"
	default:
		return errNotFound
	}
	var data []byte
	err := s.pool.QueryRow(r.Context(), `SELECT `+col+` FROM property_photos WHERE id = $1`, chi.URLParam(r, "id")).Scan(&data)
	if err == pgx.ErrNoRows || isBadUUID(err) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_, _ = w.Write(data)
	return nil
}
