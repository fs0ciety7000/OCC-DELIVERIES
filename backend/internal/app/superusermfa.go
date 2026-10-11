package app

import (
	"strconv"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// Superuser MFA: signing in to the PocketBase admin (/_/) needs the password
// AND a one-time code sent by e-mail (PocketBase MFA + OTP on _superusers).
// Applied at startup, only when e-mails can be sent (SMTP configured):
// without SMTP nothing is changed (no lock-out), with a warning if MFA is
// already on. OCC_SUPERUSER_MFA=false turns it off. Recovery (lost mailbox,
// SMTP down): `/pb/occ superuser otp <email>` prints a code in the
// container terminal; `superuser upsert <email> <password>` resets the
// password (docs/DEPLOYMENT.md).

const (
	superuserOTPLength   = 6
	superuserOTPDuration = 300 // seconds: the code
	superuserMFADuration = 600 // seconds: between the password and the code
)

// superuserMFAFromEnv reads OCC_SUPERUSER_MFA (default true).
func superuserMFAFromEnv(getenv func(string) string) bool {
	if v, err := strconv.ParseBool(strings.TrimSpace(getenv("OCC_SUPERUSER_MFA"))); err == nil {
		return v
	}
	return true
}

var superuserOTPTemplate = core.EmailTemplate{
	Subject: "Code de connexion à l'administration — {APP_NAME}",
	Body: renderMail(mailContent{
		Preheader: "Ton code pour finir de te connecter à l'administration.",
		Title:     "Connexion à l'administration",
		Paragraphs: []string{
			"Quelqu'un (toi, on l'espère) vient de saisir ton mot de passe sur l'administration de {APP_NAME}. Pour terminer la connexion, entre ce code :",
			`<strong style="font-size:26px;letter-spacing:0.18em">{OTP}</strong>`,
			"Il est valable 5 minutes et ne sert qu'une fois.",
		},
		Footnote: "Ce n'était pas toi ? Ton mot de passe d'administration est connu de quelqu'un d'autre : change-le tout de suite (et ne communique jamais ce code).",
	}),
}

// applySuperuserMFA enables (or disables) MFA + e-mail OTP on _superusers.
func applySuperuserMFA(app core.App, cfg Config) error {
	col, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		return err
	}
	changed := false
	switch {
	case !cfg.SuperuserMFA:
		// PocketBase forces MFA on whenever OTP is on for _superusers:
		// both go off together.
		if col.MFA.Enabled || col.OTP.Enabled {
			col.MFA.Enabled, col.OTP.Enabled = false, false
			changed = true
			app.Logger().Warn("superuser MFA disabled (OCC_SUPERUSER_MFA=false)")
		}
	case !mailEnabled(app):
		if col.MFA.Enabled {
			app.Logger().Warn("superuser MFA is on but SMTP is not configured: admin codes cannot be e-mailed, use `/pb/occ superuser otp <email> --dir=/pb/pb_data`")
		} else if strings.HasPrefix(cfg.PublicURL, "https://") {
			app.Logger().Warn("superuser MFA not enabled: configure SMTP (OCC_SMTP_HOST) to require an e-mailed code for /_/")
		}
	default:
		want := col.MFA
		want.Enabled, want.Rule = true, ""
		if want.Duration == 0 || want.Duration == 1800 { // PocketBase default → ours
			want.Duration = superuserMFADuration
		}
		if want != col.MFA {
			col.MFA = want
			changed = true
		}
		otp := col.OTP
		otp.Enabled, otp.Length, otp.Duration = true, superuserOTPLength, superuserOTPDuration
		otp.EmailTemplate = superuserOTPTemplate
		if otp != col.OTP {
			col.OTP = otp
			changed = true
		}
		if changed {
			app.Logger().Info("superuser MFA enabled: password + e-mailed code", "codeLength", superuserOTPLength)
		}
	}
	if !changed {
		return nil
	}
	return app.Save(col)
}
