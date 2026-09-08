package cmd

import (
	"context"
	"errors"
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
	modelImportCMD.Flags().StringVar(&modelImportSetName, "set-name", "", "Set the --card codes belong to (its real name, e.g. \"Brown Dust 2\" - see persist.GetSetIDByName). Required if --card is given.")
	modelImportCMD.Flags().StringArrayVar(&modelImportCards, "card", nil, "Card code (e.g. BRD/W139-001S) to link the imported model to (repeatable) - the whole reason this is manual: several cards commonly share one model across rarities. Requires exactly one --char and --set-name.")
	modelImportCMD.Flags().StringArrayVar(&modelImportCardIDs, "card-id", nil, "Card id (the UUID, not the code) to link the imported model to (repeatable) - skips the --set-name/code lookup entirely when you already have the id, e.g. copied from the collection UI's URL or a prior API response. Requires exactly one --char, same as --card.")
	modelImportCMD.Flags().StringVarP(&env, "env", "e", "local", "Environment to connect to")
}

var modelImportChars []string
var modelImportSetName string
var modelImportCards []string
var modelImportCardIDs []string

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
process-set uses for a malformed CSV row.

--card optionally links the imported model to one or more catalog cards
in the same run, given --set-name to say which set they belong to -
there's no separate linking command, or a picker in the collection UI
either (see fe/src/components/ui/CardModelLink.tsx's own doc comment):
with a model actually getting imported at all being this rare and
deliberate, doing the link as part of the same manual step it's already
a manual step for is simpler than a second tool. Only makes sense
alongside exactly one --char - linking is refused outright with more
than one, since there'd be no way to say which model a given --card
should point at.

  model-import --char 002406 --set-name "Brown Dust 2" \
    --card BRD/W139-001S --card BRD/W139-003S

A --card code that doesn't match a real card in --set-name is logged
and skipped, same tolerance as everything else here - the model itself
is already imported and stored by the time linking runs, so one bad
card code shouldn't be treated as if the whole command failed.

--card-id is the same idea but skips --set-name/code resolution
entirely, linking a card by its actual id (e.g. copied straight out of
the collection UI's own URL, or a GET /api/sets/{setID}/cards response) -
the more direct option when you already have it rather than the code:

  model-import --char 002406 --card-id 01900000-0000-7000-8000-000000000011`,
	Run: func(cmd *cobra.Command, args []string) {
		logger.Init(env)

		if len(modelImportChars) == 0 {
			log.Fatal().Msg("model-import requires at least one --char")
		}
		if len(modelImportCards) > 0 || len(modelImportCardIDs) > 0 {
			if len(modelImportCards) > 0 && modelImportSetName == "" {
				log.Fatal().Msg("--card requires --set-name")
			}
			if len(modelImportChars) != 1 {
				log.Fatal().Msg("--card/--card-id require exactly one --char - otherwise there's no way to say which model they should link to")
			}
		}

		db, err := persist.NewDB(env)
		if err != nil {
			log.Fatal().Err(err).Msg("error connecting to db")
		}
		p := &persist.Persist{DB: db}
		ctx := context.Background()

		modelImport(ctx, p, modelImportChars)

		if len(modelImportCards) > 0 {
			linkCardsToCharacterModel(ctx, p, modelImportSetName, modelImportChars[0], modelImportCards)
		}
		if len(modelImportCardIDs) > 0 {
			linkCardIDsToCharacterModel(ctx, p, modelImportChars[0], modelImportCardIDs)
		}
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

// linkCardsToCharacterModel resolves setName to a set (must already
// exist - unlike process-set, this never creates one) and points each of
// cardCodes at charCode via persist.SetCardCharacterModel. Doesn't stop
// on a bad card code or a resolve error partway through - see this
// command's own doc comment on --card for why.
func linkCardsToCharacterModel(ctx context.Context, p *persist.Persist, setName, charCode string, cardCodes []string) {
	setID, err := p.GetSetIDByName(ctx, setName)
	if err != nil {
		log.Error().Err(err).Str("set", setName).Msg("error resolving set, skipping all --card linking")
		return
	}

	var linked, skipped int
	for _, code := range cardCodes {
		cardID, err := p.GetCardIDByCode(ctx, setID, code)
		if err != nil {
			if errors.Is(err, persist.ErrCardNotFound) {
				log.Error().Str("code", code).Str("set", setName).Msg("no matching card for this code, skipping")
			} else {
				log.Error().Err(err).Str("code", code).Msg("error resolving card, skipping")
			}
			skipped++
			continue
		}

		if err := p.SetCardCharacterModel(ctx, cardID, &charCode); err != nil {
			log.Error().Err(err).Str("code", code).Str("charCode", charCode).Msg("error linking card, skipping")
			skipped++
			continue
		}
		linked++
		log.Info().Str("code", code).Str("charCode", charCode).Msg("linked card to character model")
	}

	log.Info().Int("linked", linked).Int("skipped", skipped).Msg("--card linking finished")
}

// linkCardIDsToCharacterModel is linkCardsToCharacterModel's simpler
// counterpart for --card-id: no set/code resolution, just
// persist.SetCardCharacterModel directly against each id - a bad id
// surfaces as persist.ErrCardNotFound from that call itself rather than
// a separate lookup step, but is logged and skipped the same way.
func linkCardIDsToCharacterModel(ctx context.Context, p *persist.Persist, charCode string, cardIDs []string) {
	var linked, skipped int
	for _, cardID := range cardIDs {
		if err := p.SetCardCharacterModel(ctx, cardID, &charCode); err != nil {
			if errors.Is(err, persist.ErrCardNotFound) {
				log.Error().Str("cardID", cardID).Msg("no matching card for this id, skipping")
			} else {
				log.Error().Err(err).Str("cardID", cardID).Msg("error linking card, skipping")
			}
			skipped++
			continue
		}
		linked++
		log.Info().Str("cardID", cardID).Str("charCode", charCode).Msg("linked card to character model")
	}

	log.Info().Int("linked", linked).Int("skipped", skipped).Msg("--card-id linking finished")
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
