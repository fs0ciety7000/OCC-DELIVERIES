package app

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/spf13/cobra"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/payoutvault"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/secretbox"
)

// Payout data at rest (ADR 0003, update 5): payout_profiles.holder_name,
// iban and bic are sealed with AES-256-GCM (internal/secretbox) when
// OCC_DATA_KEY is set. Life cycle of a value:
//
//   - request hooks see plaintext (validation / normalisation on plaintext):
//     sealed values loaded from the row are opened first;
//   - model hooks open again (any server-side Save), validation runs on
//     plaintext, the value is sealed just before the SQL write
//     (OnRecord*Execute) and the plaintext is put back in memory;
//   - API responses (collection list/view, create/update responses,
//     realtime) are opened only for the owner (OnRecordEnrich); anyone else
//     (superusers in /_/) sees the envelope;
//   - server reads (EPC QR, « Encaisser », payer guard) go through
//     openPayout, which also re-seals lazily a stale row (plaintext or old key).
//
// Without OCC_DATA_KEY everything is stored in plaintext (legacy behaviour,
// loud warning in production) and nobody is locked out.

const payoutCryptoPriority = -100 // before every other payout_profiles hook

const msgPayoutUnreadable = "Tes coordonnées bancaires enregistrées sont illisibles pour le serveur (clé de chiffrement absente ou changée). Ressaisis ton IBAN, le nom du titulaire et le BIC puis enregistre."

// dataKeysFromEnv reads OCC_DATA_KEY / OCC_DATA_KEY_OLD (never logged).
func dataKeysFromEnv(getenv func(string) string) (*secretbox.Keyring, error) {
	return secretbox.FromEnv(getenv)
}

// payoutOwnerOf is the users id a stored value was sealed for: the stored
// owner for an existing record (the owner itself is immutable for users).
func payoutOwnerOf(r *core.Record) string {
	if !r.IsNew() {
		if o := r.Original().GetString("user"); o != "" {
			return o
		}
	}
	return r.GetString("user")
}

func bindPayoutCrypto(app core.App, keys *secretbox.Keyring) {
	payoutvault.SetKeys(app, keys)

	// Request hooks: the other payout_profiles hooks validate plaintext.
	openRequest := func(e *core.RecordRequestEvent) error {
		if _, err := payoutvault.OpenRecord(payoutvault.Keys(e.App), e.Record, payoutOwnerOf(e.Record)); err != nil {
			e.App.Logger().Error("payout profile: sealed value cannot be opened", "record", e.Record.Id, "error", err)
			return apis.NewApiError(http.StatusUnprocessableEntity, msgPayoutUnreadable, nil)
		}
		return e.Next()
	}
	app.OnRecordCreateRequest(colPayoutProfiles).Bind(&hook.Handler[*core.RecordRequestEvent]{Func: openRequest, Priority: payoutCryptoPriority})
	app.OnRecordUpdateRequest(colPayoutProfiles).Bind(&hook.Handler[*core.RecordRequestEvent]{Func: openRequest, Priority: payoutCryptoPriority})

	// Model hooks: plaintext for the validators, whatever the save path.
	openModel := func(e *core.RecordEvent) error {
		if _, err := payoutvault.OpenRecord(payoutvault.Keys(e.App), e.Record, payoutOwnerOf(e.Record)); err != nil {
			return apis.NewApiError(http.StatusUnprocessableEntity, msgPayoutUnreadable, nil)
		}
		return e.Next()
	}
	app.OnRecordCreate(colPayoutProfiles).Bind(&hook.Handler[*core.RecordEvent]{Func: openModel, Priority: payoutCryptoPriority})
	app.OnRecordUpdate(colPayoutProfiles).Bind(&hook.Handler[*core.RecordEvent]{Func: openModel, Priority: payoutCryptoPriority})

	// Seal right before the SQL write, plaintext back in memory afterwards.
	sealExecute := func(e *core.RecordEvent) error {
		plain := map[string]string{}
		for _, f := range payoutvault.Fields {
			plain[f] = e.Record.GetString(f)
		}
		if err := payoutvault.SealRecord(payoutvault.Keys(e.App), e.Record, e.Record.GetString("user")); err != nil {
			return err
		}
		err := e.Next()
		for f, v := range plain {
			e.Record.Set(f, v)
		}
		return err
	}
	app.OnRecordCreateExecute(colPayoutProfiles).BindFunc(sealExecute)
	app.OnRecordUpdateExecute(colPayoutProfiles).BindFunc(sealExecute)

	// Responses: plaintext for the owner only.
	app.OnRecordEnrich(colPayoutProfiles).BindFunc(func(e *core.RecordEnrichEvent) error {
		owner := e.Record.GetString("user")
		if e.RequestInfo == nil || e.RequestInfo.Auth == nil || e.RequestInfo.Auth.Id != owner ||
			e.RequestInfo.Auth.Collection().Name != colUsers {
			return e.Next()
		}
		if _, err := payoutvault.OpenRecord(payoutvault.Keys(e.App), e.Record, owner); err != nil {
			// never hand an envelope to the form: the owner re-enters the data
			e.App.Logger().Error("payout profile: sealed value cannot be opened", "record", e.Record.Id, "error", err)
			for _, f := range payoutvault.Fields {
				if secretbox.IsSealed(e.Record.GetString(f)) {
					e.Record.Set(f, "")
				}
			}
		}
		return e.Next()
	})
}

// openPayout returns a plaintext copy of a payout profile for server-side
// use (EPC payload, payer guard). Unreadable fields are blanked (logged).
// A stale row (plaintext, or sealed with OCC_DATA_KEY_OLD) is re-sealed
// with the current key on the way.
func openPayout(app core.App, prof *core.Record) *core.Record {
	if prof == nil {
		return nil
	}
	keys := payoutvault.Keys(app)
	stored := map[string]string{}
	for _, f := range payoutvault.Fields {
		stored[f] = prof.GetString(f)
	}
	out := prof.Clone()
	owner := prof.GetString("user")
	stale, err := payoutvault.OpenRecord(keys, out, owner)
	if err != nil {
		app.Logger().Error("payout profile: sealed value cannot be opened", "record", prof.Id, "error", err)
		for _, f := range payoutvault.Fields {
			if secretbox.IsSealed(out.GetString(f)) {
				out.Set(f, "")
			}
		}
		return out
	}
	if stale && keys.Enabled() {
		plain := map[string]string{}
		for _, f := range payoutvault.Fields {
			plain[f] = out.GetString(f)
		}
		if _, err := payoutvault.Reseal(app, keys, prof.Id, owner, stored, plain); err != nil {
			app.Logger().Warn("payout profile: lazy re-seal failed", "record", prof.Id, "error", err)
		}
	}
	return out
}

// checkPayoutEncryption runs at startup: refuses an invalid key, warns
// loudly when production stores bank data in plaintext, seals what is
// still plaintext (or sealed with an old key) and reports unreadable rows.
func checkPayoutEncryption(app core.App, cfg Config) error {
	if cfg.DataKeyErr != nil {
		// the error names the variable, never its value
		return fmt.Errorf("invalid payout encryption key: %w", cfg.DataKeyErr)
	}
	keys := payoutvault.Keys(app)
	if !keys.Enabled() {
		if strings.HasPrefix(cfg.PublicURL, "https://") {
			app.Logger().Warn("!!! OCC_DATA_KEY is not set: payout data (IBAN, account holder, BIC) is stored IN PLAINTEXT. " +
				"Generate a key (openssl rand -base64 32), set OCC_DATA_KEY and restart — see docs/DEPLOYMENT.md.")
		} else {
			app.Logger().Info("OCC_DATA_KEY not set: payout data stored in plaintext (development)")
		}
	}
	rep, err := payoutvault.Sweep(app, keys)
	if err != nil {
		return fmt.Errorf("payout encryption sweep: %w", err)
	}
	if rep.Updated > 0 {
		app.Logger().Info("payout profiles sealed with the current OCC_DATA_KEY", "updated", rep.Updated, "keyId", keys.CurrentID())
	}
	if rep.Failed > 0 {
		app.Logger().Error("!!! payout profiles sealed with an unknown key cannot be read: set OCC_DATA_KEY (and OCC_DATA_KEY_OLD during a rotation) to the key used to seal them",
			"rows", rep.Failed)
	}
	return nil
}

// NewPayoutsReencryptCommand is `/pb/occ payouts-reencrypt`: one-shot sweep
// that seals every payout field still in plaintext or sealed with an old key
// (OCC_DATA_KEY_OLD) with the current OCC_DATA_KEY. Run it after a rotation,
// then remove OCC_DATA_KEY_OLD. `--decrypt` writes everything back in
// plaintext (turning encryption off; needs the keys that sealed the data).
func NewPayoutsReencryptCommand(app core.App, getenv func(string) string) *cobra.Command {
	var decrypt bool
	cmd := &cobra.Command{
		Use:          "payouts-reencrypt",
		Short:        "Seal payout data (IBAN, holder, BIC) with OCC_DATA_KEY (rotation from OCC_DATA_KEY_OLD)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			keys, err := dataKeysFromEnv(getenv)
			if err != nil {
				return err
			}
			var rep payoutvault.Report
			if decrypt {
				if !keys.HasKeys() {
					return errors.New("--decrypt needs OCC_DATA_KEY and/or OCC_DATA_KEY_OLD")
				}
				rep, err = payoutvault.Decrypt(app, keys)
			} else {
				if !keys.Enabled() {
					return errors.New("OCC_DATA_KEY is not set")
				}
				rep, err = payoutvault.Sweep(app, keys)
			}
			if err != nil {
				return err
			}
			cmd.Printf("payout profiles: %d read, %d updated, %d sealed, %d plaintext, %d unreadable (key id %s)\n",
				rep.Rows, rep.Updated, rep.Sealed, rep.Plain, rep.Failed, keys.CurrentID())
			if rep.Failed > 0 {
				return fmt.Errorf("%d payout profiles sealed with an unknown key: add it to OCC_DATA_KEY_OLD", rep.Failed)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&decrypt, "decrypt", false, "write every payout field back in plaintext")
	return cmd
}
