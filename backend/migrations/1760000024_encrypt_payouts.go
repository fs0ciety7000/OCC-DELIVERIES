package migrations

import (
	"os"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/payoutvault"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/secretbox"
)

// Payout data at rest (ADR 0003, update 5): payout_profiles.holder_name,
// iban and bic are sealed with AES-256-GCM under OCC_DATA_KEY
// (enc:v1:<kid>:<base64>, internal/secretbox). This migration seals the
// existing plaintext rows when the key is set. Without the key it changes
// nothing (no crash, nobody locked out): the server then warns at startup
// and seals the rows as soon as the key appears (startup sweep in
// internal/app/payoutcrypto.go, same code). Idempotent: sealed values are
// skipped. An invalid OCC_DATA_KEY fails the migration (never silently
// ignored). No schema change: validation still runs on plaintext.

func init() {
	m.Register(upEncryptPayouts, downEncryptPayouts)
}

// dataKeysEnv is os.Getenv, replaceable in tests.
var dataKeysEnv = os.Getenv

func upEncryptPayouts(app core.App) error {
	keys, err := secretbox.FromEnv(dataKeysEnv)
	if err != nil {
		return err
	}
	if !keys.Enabled() {
		app.Logger().Warn("1760000024: OCC_DATA_KEY not set, payout profiles left in plaintext")
		return nil
	}
	rep, err := payoutvault.Sweep(app, keys)
	if err != nil {
		return err
	}
	app.Logger().Info("1760000024: payout profiles sealed", "updated", rep.Updated, "unreadable", rep.Failed)
	return nil
}

// downEncryptPayouts writes the values back in plaintext when the keys are
// available (rows sealed with an unknown key are left as they are).
func downEncryptPayouts(app core.App) error {
	keys, err := secretbox.FromEnv(dataKeysEnv)
	if err != nil {
		return err
	}
	if !keys.HasKeys() {
		return nil
	}
	_, err = payoutvault.Decrypt(app, keys)
	return err
}
