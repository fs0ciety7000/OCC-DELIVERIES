// Package payoutvault applies the at-rest encryption of internal/secretbox
// to the payout_profiles bank fields (holder_name, iban, bic). It is shared
// by the app hooks, the startup sweep, the CLI command and the migration
// 1760000024 (see docs/ARCHITECTURE.md, payout_profiles, and ADR 0003,
// update 5).
//
// revolut_tag / paypal_me / payment_link are NOT encrypted: they are handles
// meant to be shared (printed on revolut.me / paypal.me pages, sent to
// anyone asking for money), carry no bank account number, and stay readable
// for the payer guard without any key.
package payoutvault

import (
	"errors"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/secretbox"
)

// Collection is the encrypted collection.
const Collection = "payout_profiles"

// Fields are the encrypted columns of payout_profiles.
var Fields = []string{"holder_name", "iban", "bic"}

const storeKey = "occ.payoutvault.keys"

// ErrUnreadable is returned (wrapped) when a sealed field cannot be opened.
var ErrUnreadable = errors.New("payoutvault: sealed payout field cannot be opened")

// AAD binds a sealed value to its column and owner (users id).
func AAD(field, owner string) string { return Collection + "." + field + ":" + owner }

// SetKeys stores the keyring in the app store (shared by transactions).
func SetKeys(app core.App, k *secretbox.Keyring) { app.Store().Set(storeKey, k) }

// Keys returns the keyring of the app (nil = no key).
func Keys(app core.App) *secretbox.Keyring {
	k, _ := app.Store().Get(storeKey).(*secretbox.Keyring)
	return k
}

// OpenRecord replaces, in place, every sealed field of rec by its plaintext
// (owner = users id the values were sealed for). Plaintext fields are kept.
// stale reports that at least one field should be sealed again with the
// current key. On error the record is left partially opened: every field
// that could not be opened keeps its envelope.
func OpenRecord(k *secretbox.Keyring, rec *core.Record, owner string) (stale bool, err error) {
	var errs []error
	for _, f := range Fields {
		plain, st, e := k.Open(rec.GetString(f), AAD(f, owner))
		if e != nil {
			errs = append(errs, fmt.Errorf("%w: %s: %w", ErrUnreadable, f, e))
			continue
		}
		stale = stale || st
		rec.Set(f, plain)
	}
	return stale, errors.Join(errs...)
}

// SealRecord replaces, in place, every plaintext field of rec by its
// envelope (no-op without a current key). Already sealed values are an error.
func SealRecord(k *secretbox.Keyring, rec *core.Record, owner string) error {
	for _, f := range Fields {
		s, err := k.Seal(rec.GetString(f), AAD(f, owner))
		if err != nil {
			return fmt.Errorf("payoutvault: %s: %w", f, err)
		}
		rec.Set(f, s)
	}
	return nil
}

// row is one payout_profiles row read with raw SQL.
type row struct {
	ID         string `db:"id"`
	User       string `db:"user"`
	HolderName string `db:"holder_name"`
	IBAN       string `db:"iban"`
	BIC        string `db:"bic"`
}

func (r row) values() map[string]string {
	return map[string]string{"holder_name": r.HolderName, "iban": r.IBAN, "bic": r.BIC}
}

// Reseal writes, with raw SQL (no hooks, `updated` untouched), the given
// plaintext values sealed with the current key — only if the row still
// holds the `stored` values (a concurrent edit wins). No-op without key.
func Reseal(app core.App, k *secretbox.Keyring, id, owner string, stored, plain map[string]string) (bool, error) {
	if !k.Enabled() {
		return false, nil
	}
	set := dbx.Params{}
	where := dbx.HashExp{"id": id}
	for _, f := range Fields {
		s, err := k.Seal(plain[f], AAD(f, owner))
		if err != nil {
			return false, err
		}
		set[f] = s
		where[f] = stored[f]
	}
	res, err := app.NonconcurrentDB().Update(Collection, set, where).Execute()
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Report sums up a sweep.
type Report struct {
	Rows int // payout profiles read
	// Updated rows: sealed (plaintext or older key → current key), or
	// written back in plaintext by Decrypt.
	Updated int
	// Failed rows: at least one sealed field cannot be opened with the
	// configured keys (left untouched).
	Failed int
	// Sealed / Plain rows after the sweep (rows with at least one sealed
	// field / rows with data and no sealed field).
	Sealed, Plain int
}

// Sweep seals with the current key every payout field still in plaintext
// or sealed with an older key (OCC_DATA_KEY_OLD). Idempotent; a no-op
// write-wise without current key (it then only counts rows). Runs at
// startup, in the migration 1760000024 and in `occ payouts-reencrypt`.
func Sweep(app core.App, k *secretbox.Keyring) (Report, error) {
	return sweep(app, k, false)
}

// Decrypt writes every readable sealed payout field back in plaintext
// (down migration, or turning encryption off). Rows that cannot be opened
// are left untouched and counted in Failed.
func Decrypt(app core.App, k *secretbox.Keyring) (Report, error) {
	return sweep(app, k, true)
}

func sweep(app core.App, k *secretbox.Keyring, toPlain bool) (Report, error) {
	var rep Report
	var rows []row
	if err := app.DB().Select("id", "user", "holder_name", "iban", "bic").From(Collection).All(&rows); err != nil {
		return rep, err
	}
	rep.Rows = len(rows)
	err := app.RunInTransaction(func(tx core.App) error {
		for _, r := range rows {
			stored := r.values()
			plain := map[string]string{}
			stale, failed, sealed, hasData := false, false, false, false
			for _, f := range Fields {
				v := stored[f]
				hasData = hasData || v != ""
				sealed = sealed || secretbox.IsSealed(v)
				p, st, err := k.Open(v, AAD(f, r.User))
				if err != nil {
					failed = true
					break
				}
				plain[f] = p
				stale = stale || st
			}
			switch {
			case failed:
				rep.Failed++
				rep.Sealed++
				continue
			case toPlain && sealed:
				set, where := dbx.Params{}, dbx.HashExp{"id": r.ID}
				for _, f := range Fields {
					set[f], where[f] = plain[f], stored[f]
				}
				if _, err := tx.NonconcurrentDB().Update(Collection, set, where).Execute(); err != nil {
					return err
				}
				rep.Updated++
				sealed = false
			case !toPlain && stale:
				done, err := Reseal(tx, k, r.ID, r.User, stored, plain)
				if err != nil {
					return err
				}
				if done {
					rep.Updated++
					sealed = true
				}
			}
			if sealed {
				rep.Sealed++
			} else if hasData {
				rep.Plain++
			}
		}
		return nil
	})
	return rep, err
}
