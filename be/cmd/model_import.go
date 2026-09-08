package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"example.com/mishis4x/logger"
	"example.com/mishis4x/persist"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// modelAssetBaseURL is jelosus2/BD2-L2D-Viewer's own GitHub Pages asset
// path (https://github.com/jelosus2/BD2-L2D-Viewer) - a fan-made viewer
// for Brown Dust 2's Spine character models, unofficially extracted from
// the game client. Confirmed by inspecting the viewer's real network
// traffic: every character's three files live at a predictable
// {base}/{charCode}/char{charCode}.{skel,atlas,png}, no auth, no signed
// URLs, unrelated to the anim/skin/type query params the viewer's own UI
// takes (those only pick what its JS plays client-side from the same
// fixed file set).
//
// This is intentionally a hardcoded allowlist-driven CLI step (see this
// command's own doc comment), not a user-facing or automatic import - the
// assets themselves are Neowiz's copyrighted game content, not licensed
// to the viewer or to this app.
// A var, not a const, solely so tests can point it at a fake server (see
// setModelAssetBaseURLForTest in model_import_test.go) - never reassigned
// outside tests.
var modelAssetBaseURL = "https://jelosus2.github.io/BD2-L2D-Viewer/assets/spines"

// modelDownloadTimeout is per-file, matching ebay/email's own per-request
// timeout convention - three files per character, each gets its own
// budget rather than one timeout shared across all of them.
const modelDownloadTimeout = 30 * time.Second

func init() {
	rootCMD.AddCommand(modelImportCMD)
	modelImportCMD.Flags().StringArrayVarP(&modelImportChars, "char", "c", nil, "Character code to import (repeatable, e.g. -c 002406 -c 002407)")
	modelImportCMD.Flags().StringVarP(&env, "env", "e", "local", "Environment to connect to")
}

var modelImportChars []string

var modelImportCMD = &cobra.Command{
	Use:   "model-import",
	Short: "Download and store Spine character model assets for a hand-picked list of char codes",
	Long: `Download a character's Spine model assets (skeleton, atlas, texture) from
jelosus2's BD2-L2D-Viewer and store them in character_models.

--char is repeatable and takes the viewer's own 6-digit internal
character ID (visible in its URL as ?char=002406) - there's no
auto-discovery or "import everything" mode, by design: these are
unofficially-extracted, copyrighted game assets, so this command only
ever fetches char codes someone has deliberately chosen, one at a time.

  model-import --char 002406 --char 002407

Re-running against an already-imported char code overwrites its stored
assets (ON DUPLICATE KEY UPDATE) - safe to re-run if the source viewer
ever updates a model.

A single char code's download failure (network error, unexpected status,
one of the three files missing) is logged and skipped rather than
aborting the rest of the list - the same per-item tolerance
process-set uses for a malformed CSV row.`,
	Run: func(cmd *cobra.Command, args []string) {
		logger.Init(env)

		if len(modelImportChars) == 0 {
			log.Fatal().Msg("model-import requires at least one --char")
		}

		db, err := persist.NewDB(env)
		if err != nil {
			log.Fatal().Err(err).Msg("error connecting to db")
		}
		p := &persist.Persist{DB: db}

		modelImport(context.Background(), p, modelImportChars)
	},
}

func modelImport(ctx context.Context, p *persist.Persist, charCodes []string) {
	client := &http.Client{Timeout: modelDownloadTimeout}

	var imported, skipped int
	for _, charCode := range charCodes {
		if err := importCharacterModel(ctx, client, p, charCode); err != nil {
			log.Error().Err(err).Str("charCode", charCode).Msg("error importing character model, skipping")
			skipped++
			continue
		}
		imported++
		log.Info().Str("charCode", charCode).Msg("imported character model")
	}

	log.Info().Int("imported", imported).Int("skipped", skipped).Msg("model-import finished")
}

// importCharacterModel downloads all three of one character's asset files
// and stores them together. Fails as a whole (nothing partially stored)
// if any one of the three can't be fetched - a model missing its texture
// or atlas isn't renderable, so there's no useful partial state to keep.
func importCharacterModel(ctx context.Context, client *http.Client, p *persist.Persist, charCode string) error {
	skeleton, err := downloadModelAsset(ctx, client, charCode, "skel")
	if err != nil {
		return fmt.Errorf("skeleton: %w", err)
	}

	atlas, err := downloadModelAsset(ctx, client, charCode, "atlas")
	if err != nil {
		return fmt.Errorf("atlas: %w", err)
	}

	texture, err := downloadModelAsset(ctx, client, charCode, "png")
	if err != nil {
		return fmt.Errorf("texture: %w", err)
	}

	return p.UpsertCharacterModel(ctx, charCode, skeleton, atlas, texture, http.DetectContentType(texture))
}

// downloadModelAsset fetches one file for charCode - ext is "skel",
// "atlas", or "png", matching the source viewer's own char{charCode}.{ext}
// naming.
func downloadModelAsset(ctx context.Context, client *http.Client, charCode, ext string) ([]byte, error) {
	url := fmt.Sprintf("%s/%s/char%s.%s", modelAssetBaseURL, charCode, charCode, ext)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	return data, nil
}
