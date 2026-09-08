package handlers

import (
	"bytes"
	"context"
	"errors"
	"net/http"

	"example.com/mishis4x/persist"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// ListCharacterModels returns every char_code the model-import CLI command
// has stored, for the frontend's model picker - just the codes, same
// "list what's available, not the blobs" shape as ListSets.
func (d *Data) ListCharacterModels(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	codes, err := d.P.ListCharacterModels(ctx)
	if err != nil {
		log.Error().Err(err).Msg("error listing character models")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	writeJSON(w, http.StatusOK, codes)
}

// GetCharacterModelSkeleton streams the stored .skel bytes for the
// character named by the {charCode} path variable. Served as
// application/octet-stream - Spine's runtime loaders read this as raw
// binary, not something with a meaningful browser-native content type.
func (d *Data) GetCharacterModelSkeleton(w http.ResponseWriter, r *http.Request) {
	serveCharacterModelAsset(w, r, d.P, func(m persist.CharacterModel) ([]byte, string) {
		return m.Skeleton, "application/octet-stream"
	})
}

// GetCharacterModelAtlas streams the stored .atlas bytes for the character
// named by the {charCode} path variable. Served as text/plain - Spine
// atlas files are a human-readable text format, and the runtime's atlas
// loader reads them as text.
func (d *Data) GetCharacterModelAtlas(w http.ResponseWriter, r *http.Request) {
	serveCharacterModelAsset(w, r, d.P, func(m persist.CharacterModel) ([]byte, string) {
		return m.Atlas, "text/plain; charset=utf-8"
	})
}

// GetCharacterModelTexture streams the stored texture image for the
// character named by the {charCode} path variable, using the content type
// recorded at import time (see UpsertCharacterModel).
func (d *Data) GetCharacterModelTexture(w http.ResponseWriter, r *http.Request) {
	serveCharacterModelAsset(w, r, d.P, func(m persist.CharacterModel) ([]byte, string) {
		return m.Texture, m.TextureContentType
	})
}

// serveCharacterModelAsset is the shared lookup/streaming path for all
// three model-asset endpoints above - each only differs in which field of
// the stored CharacterModel it serves and under what content type, passed
// in via pick. 404s (as a plain string, not JSON - this endpoint's success
// response isn't JSON either, same convention as GetCardImage) if
// {charCode} was never imported, which is the ordinary state for anything
// outside the model-import CLI command's hand-picked allowlist.
func serveCharacterModelAsset(w http.ResponseWriter, r *http.Request, p persist.Persist, pick func(persist.CharacterModel) ([]byte, string)) {
	charCode := mux.Vars(r)["charCode"]

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	model, err := p.GetCharacterModel(ctx, charCode)
	if err != nil {
		if errors.Is(err, persist.ErrCharacterModelNotFound) {
			http.Error(w, "Model not found.", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("charCode", charCode).Msg("error getting character model")
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}

	data, contentType := pick(model)

	// These only change on a deliberate model-import re-run (see
	// UpsertCharacterModel), same infrequent-but-not-never cadence as
	// card_images - a full day of blind caching plus Last-Modified-based
	// revalidation, same convention as GetCardImage.
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, "", model.UpdatedAt, bytes.NewReader(data))
}
