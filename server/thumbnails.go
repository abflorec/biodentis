package main

import (
	"bytes"
	"context"
	"database/sql"
	"image"
	"image/jpeg"
	"net/http"
)

const thumbnailSide = 320
const thumbnailMaxPixels = 16_000_000

func (s *server) downloadRadiographThumbnail(w http.ResponseWriter, r *http.Request) {
	id, ok := careID(w, r)
	if !ok {
		return
	}
	data, err := s.radiographThumbnail(r.Context(), id)
	if err != nil {
		if r.Context().Err() == nil {
			careRowError(w, err, "No encontramos la radiografía.")
		}
		return
	}
	if len(data) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Write(data)
}

func (s *server) findThumbnail(ctx context.Context, id int64) (string, []byte, bool, error) {
	var mimeType string
	var data []byte
	var cachedID sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT r.mime, t.data, t.radiograph_id FROM radiographs r
		LEFT JOIN radiograph_thumbnails t ON t.radiograph_id=r.id WHERE r.id=?`, id).
		Scan(&mimeType, &data, &cachedID)
	return mimeType, data, cachedID.Valid, err
}

func (s *server) radiographThumbnail(ctx context.Context, id int64) ([]byte, error) {
	mimeType, data, cached, err := s.findThumbnail(ctx, id)
	if err != nil || cached || mimeType == "application/pdf" {
		return data, err
	}

	// Bound peak memory when a gallery requests several uncached images at once.
	s.thumbnailMu.Lock()
	defer s.thumbnailMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_, data, cached, err = s.findThumbnail(ctx, id)
	if err != nil || cached {
		return data, err
	}
	var original []byte
	if err := s.db.QueryRowContext(ctx, "SELECT data FROM radiographs WHERE id=?", id).Scan(&original); err != nil {
		return nil, err
	}
	data = makeThumbnail(original)
	// A NULL cache entry also prevents repeated decoding of unsupported images.
	_, err = s.db.ExecContext(ctx, "INSERT INTO radiograph_thumbnails(radiograph_id, data) VALUES (?, ?) ON CONFLICT DO NOTHING", id, data)
	return data, err
}

func makeThumbnail(original []byte) []byte {
	config, _, err := image.DecodeConfig(bytes.NewReader(original))
	if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > thumbnailMaxPixels {
		return nil
	}
	source, _, err := image.Decode(bytes.NewReader(original))
	if err != nil {
		return nil
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if longest := max(width, height); longest > thumbnailSide {
		width = max(1, width*thumbnailSide/longest)
		height = max(1, height*thumbnailSide/longest)
	}
	preview := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			sourceX := bounds.Min.X + (2*x+1)*bounds.Dx()/(2*width)
			sourceY := bounds.Min.Y + (2*y+1)*bounds.Dy()/(2*height)
			preview.Set(x, y, source.At(sourceX, sourceY))
		}
	}
	var result bytes.Buffer
	if err := jpeg.Encode(&result, preview, &jpeg.Options{Quality: 80}); err != nil {
		return nil
	}
	return result.Bytes()
}
