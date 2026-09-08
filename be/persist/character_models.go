package persist

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
)

// ErrCharacterModelNotFound is returned by GetCharacterModel when charCode
// has no stored model - an unimported char_code is the ordinary state for
// anything outside the model-import CLI command's hand-picked allowlist,
// not a data-integrity problem (see character_models' own migration for
// why this table is only ever populated by that one command).
var ErrCharacterModelNotFound = errors.New("character model not found")

// CharacterModel is one character's full Spine asset set, as returned by
// GetCharacterModel. TextureContentType is stored alongside the texture
// bytes (mirroring card_images' image/content_type pairing) since the
// source viewer serves plain .png files with no other way to recover the
// MIME type at request time.
type CharacterModel struct {
	Skeleton           []byte
	Atlas              []byte
	Texture            []byte
	TextureContentType string
	UpdatedAt          time.Time
}

// UpsertCharacterModel stores (or replaces, on a re-run of model-import
// against an already-imported char_code) the full asset set for charCode -
// safe to call repeatedly, same re-run safety as UpsertCardImage.
func (p *Persist) UpsertCharacterModel(ctx context.Context, charCode string, skeleton, atlas, texture []byte, textureContentType string) error {
	_, err := sq.Insert("character_models").
		Columns("char_code", "skeleton", "atlas", "texture", "texture_content_type").
		Values(charCode, skeleton, atlas, texture, textureContentType).
		Suffix("ON DUPLICATE KEY UPDATE skeleton = VALUES(skeleton), atlas = VALUES(atlas), texture = VALUES(texture), texture_content_type = VALUES(texture_content_type)").
		RunWith(p.DB).
		ExecContext(ctx)
	return err
}

// GetCharacterModel returns the stored asset set for charCode. Returns
// ErrCharacterModelNotFound if charCode hasn't been imported, rather than
// a zero-value CharacterModel indistinguishable from a real (if
// nonsensical) empty one.
func (p *Persist) GetCharacterModel(ctx context.Context, charCode string) (CharacterModel, error) {
	row := sq.Select("skeleton", "atlas", "texture", "texture_content_type", "updated_at").
		From("character_models").
		Where(sq.Eq{"char_code": charCode}).
		RunWith(p.DB).
		QueryRowContext(ctx)

	var m CharacterModel
	if err := row.Scan(&m.Skeleton, &m.Atlas, &m.Texture, &m.TextureContentType, &m.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CharacterModel{}, ErrCharacterModelNotFound
		}
		return CharacterModel{}, err
	}

	return m, nil
}

// ListCharacterModels returns every imported char_code, sorted, for the
// model picker (GetGlobalData/an admin-only listing endpoint) - just the
// keys, never the blobs themselves, since a picker only needs to know
// what's available.
func (p *Persist) ListCharacterModels(ctx context.Context) ([]string, error) {
	rows, err := sq.Select("char_code").
		From("character_models").
		OrderBy("char_code").
		RunWith(p.DB).
		QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}

	return codes, rows.Err()
}

// SetCardCharacterModel links cardID to charCode (or clears the link, if
// charCode is nil) - see cards.character_model_char_code's own migration
// for why this is a plain nullable column rather than a join table (a
// card never needs more than one model at a time, even though several
// cards commonly share the same one across rarities). Returns
// ErrCardNotFound if cardID doesn't match a real card.
//
// Existence is checked as a separate SELECT rather than trusting the
// UPDATE's own RowsAffected==0, unlike ConsumePasswordReset/RedeemInvite's
// WHERE-clause-as-concurrency-guard pattern elsewhere in this package:
// this connection doesn't set CLIENT_FOUND_ROWS, so MySQL's UPDATE only
// counts rows whose value actually *changed* - re-setting a card to the
// char_code it's already linked to (or clearing an already-clear link,
// e.g. a retried request) would otherwise misreport as "card not found"
// even though the card is right there. A charCode that doesn't match any
// imported character_models row still fails loudly via the column's own
// FK constraint on the UPDATE itself - not checked separately here, same
// "let the database enforce it" convention as UpsertCard's set_id FK.
func (p *Persist) SetCardCharacterModel(ctx context.Context, cardID string, charCode *string) error {
	var exists int
	err := sq.Select("1").From("cards").Where(sq.Eq{"id": cardID}).
		RunWith(p.DB).QueryRowContext(ctx).Scan(&exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCardNotFound
		}
		return err
	}

	_, err = sq.Update("cards").
		Set("character_model_char_code", charCode).
		Where(sq.Eq{"id": cardID}).
		RunWith(p.DB).
		ExecContext(ctx)
	return err
}
