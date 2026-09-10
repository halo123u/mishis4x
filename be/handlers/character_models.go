package handlers

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strconv"

	"example.com/mishis4x/api"
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

// GetCharacterModelAudio streams one stored voice-line clip for the
// character named by the {charCode} path variable, at the {language}
// (e.g. "JP", "KR") and {clipIndex} (1-based) path variables. The
// frontend is expected to probe clip_index 1, 2, 3 and stop at the
// first 404 rather than calling a separate "list clips" endpoint first -
// see GetCharacterModelAudioClip's own doc comment for why that's a
// reasonable default here. A non-numeric {clipIndex} 400s rather than
// panicking - real input from an untrusted client, same convention as
// admin.go's inviteIDFromPath.
func (d *Data) GetCharacterModelAudio(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	charCode := vars["charCode"]
	language := vars["language"]

	clipIndex, err := strconv.Atoi(vars["clipIndex"])
	if err != nil {
		http.Error(w, "Invalid clip index.", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	clip, err := d.P.GetCharacterModelAudioClip(ctx, charCode, language, clipIndex)
	if err != nil {
		if errors.Is(err, persist.ErrCharacterModelAudioNotFound) {
			http.Error(w, "Audio clip not found.", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("charCode", charCode).Str("language", language).Int("clipIndex", clipIndex).Msg("error getting character model audio clip")
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}

	// Same caching convention as serveCharacterModelAsset - these only
	// change on a deliberate model-import re-run.
	w.Header().Set("Content-Type", clip.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, "", clip.UpdatedAt, bytes.NewReader(clip.Audio))
}

// SetCardCharacterModel links (or unlinks, if char_code is null) the card
// named by the {cardID} path variable to a character model - see
// api.SetCardCharacterModelInput's doc comment on the request body, and
// cards.character_model_char_code's own migration for why this is a
// plain nullable column on cards rather than a join table. Gated by
// modelOnlyMiddleware like every other /api/models/... route, even
// though it's cards this actually writes to - the ability to point a
// catalog card at a character model only has meaning for whoever can
// already see those models.
func (d *Data) SetCardCharacterModel(w http.ResponseWriter, r *http.Request) {
	cardID := mux.Vars(r)["cardID"]

	var body api.SetCardCharacterModelInput
	if !decodeJSONBody(w, r, &body) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	if err := d.P.SetCardCharacterModel(ctx, cardID, body.CharCode); err != nil {
		if errors.Is(err, persist.ErrCardNotFound) {
			writeJSONError(w, http.StatusNotFound, "Card not found.")
			return
		}
		// A charCode that doesn't match any imported character_models row
		// fails here too, via the column's FK constraint - genuinely rare
		// (the frontend only ever offers already-imported codes) and not
		// worth a more specific status than 500 for.
		log.Error().Err(err).Str("cardID", cardID).Msg("error setting card's character model")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	w.WriteHeader(http.StatusOK)
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
