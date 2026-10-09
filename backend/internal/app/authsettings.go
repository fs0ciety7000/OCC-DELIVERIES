package app

import (
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// MailConfig is the outgoing mail configuration (OCC_SMTP_*, OCC_MAIL_*).
// Applied to the PocketBase settings at startup only when OCC_SMTP_HOST is
// set: otherwise what the superuser configured in /_/ is left untouched.
type MailConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	// TLS = implicit TLS (port 465). false = STARTTLS when the server offers it.
	TLS      bool
	From     string
	FromName string
}

// GoogleConfig holds the Google OAuth2 client (OCC_GOOGLE_CLIENT_ID / _SECRET).
type GoogleConfig struct {
	ClientID     string
	ClientSecret string
}

// Enabled reports whether both values are set.
func (g GoogleConfig) Enabled() bool { return g.ClientID != "" && g.ClientSecret != "" }

const (
	defaultMailFromName = "OCC Deliveries"
	googleProvider      = "google"
)

// mailConfigFromEnv reads OCC_SMTP_* / OCC_MAIL_* (lookup = os.Getenv).
func mailConfigFromEnv(lookup func(string) string) MailConfig {
	get := func(k string) string { return strings.TrimSpace(lookup(k)) }
	mc := MailConfig{
		Host:     get("OCC_SMTP_HOST"),
		Port:     587,
		Username: get("OCC_SMTP_USERNAME"),
		Password: lookup("OCC_SMTP_PASSWORD"), // kept verbatim
		From:     get("OCC_MAIL_FROM"),
		FromName: get("OCC_MAIL_FROM_NAME"),
	}
	if p, err := strconv.Atoi(get("OCC_SMTP_PORT")); err == nil && p > 0 && p < 65536 {
		mc.Port = p
	}
	// implicit TLS on 465 unless told otherwise; STARTTLS (auto) elsewhere
	mc.TLS = mc.Port == 465
	if v, err := strconv.ParseBool(get("OCC_SMTP_TLS")); err == nil {
		mc.TLS = v
	}
	if mc.From == "" && strings.Contains(mc.Username, "@") {
		mc.From = mc.Username
	}
	if mc.FromName == "" {
		mc.FromName = defaultMailFromName
	}
	return mc
}

func googleConfigFromEnv(lookup func(string) string) GoogleConfig {
	return GoogleConfig{
		ClientID:     strings.TrimSpace(lookup("OCC_GOOGLE_CLIENT_ID")),
		ClientSecret: strings.TrimSpace(lookup("OCC_GOOGLE_CLIENT_SECRET")),
	}
}

func authConfigFromEnv(cfg *Config) {
	cfg.Mail = mailConfigFromEnv(os.Getenv)
	cfg.Google = googleConfigFromEnv(os.Getenv)
}

// applyMailSettings copies the env mail configuration into the settings.
// Returns whether something changed. Nothing is touched without a host.
func applyMailSettings(s *core.Settings, mc MailConfig, publicURL string) bool {
	changed := false
	set := func(dst *string, v string) {
		if v != "" && *dst != v {
			*dst = v
			changed = true
		}
	}
	if u := strings.TrimRight(publicURL, "/"); u != "" {
		if _, err := url.ParseRequestURI(u); err == nil {
			set(&s.Meta.AppURL, u)
		}
	}
	if mc.Host == "" {
		return changed
	}
	if mc.From != "" {
		if _, err := mail.ParseAddress(mc.From); err == nil {
			set(&s.Meta.SenderAddress, mc.From)
		}
	}
	set(&s.Meta.SenderName, mc.FromName)
	smtp := s.SMTP
	smtp.Enabled = true
	smtp.Host = mc.Host
	smtp.Port = mc.Port
	smtp.Username = mc.Username
	smtp.Password = mc.Password
	smtp.TLS = mc.TLS
	if smtp.AuthMethod == "" {
		smtp.AuthMethod = "PLAIN"
	}
	if smtp != s.SMTP {
		s.SMTP = smtp
		changed = true
	}
	return changed
}

// applyGoogleOAuth enables the Google provider on the users collection with
// the env credentials and maps the Google name / picture to name / avatar.
// Without credentials nothing is touched (configuration made in /_/ stays).
func applyGoogleOAuth(col *core.Collection, g GoogleConfig) bool {
	if !g.Enabled() {
		return false
	}
	changed := false
	if !col.OAuth2.Enabled {
		col.OAuth2.Enabled = true
		changed = true
	}
	found := false
	for i, p := range col.OAuth2.Providers {
		if p.Name != googleProvider {
			continue
		}
		found = true
		if p.ClientId != g.ClientID || p.ClientSecret != g.ClientSecret {
			col.OAuth2.Providers[i].ClientId = g.ClientID
			col.OAuth2.Providers[i].ClientSecret = g.ClientSecret
			changed = true
		}
	}
	if !found {
		col.OAuth2.Providers = append(col.OAuth2.Providers, core.OAuth2ProviderConfig{
			Name: googleProvider, ClientId: g.ClientID, ClientSecret: g.ClientSecret,
		})
		changed = true
	}
	if col.OAuth2.MappedFields.Name == "" {
		col.OAuth2.MappedFields.Name = "name"
		changed = true
	}
	if col.OAuth2.MappedFields.AvatarURL == "" {
		col.OAuth2.MappedFields.AvatarURL = "avatar"
		changed = true
	}
	return changed
}

// applyAuthTemplates installs the French e-mail templates (links to the SPA).
func applyAuthTemplates(col *core.Collection) bool {
	changed := false
	set := func(dst *core.EmailTemplate, t core.EmailTemplate) {
		if *dst != t {
			*dst = t
			changed = true
		}
	}
	set(&col.VerificationTemplate, verificationTemplate)
	set(&col.ResetPasswordTemplate, resetPasswordTemplate)
	set(&col.ConfirmEmailChangeTemplate, confirmEmailChangeTemplate)
	set(&col.AuthAlert.EmailTemplate, authAlertTemplate)
	set(&col.OTP.EmailTemplate, otpTemplate)
	return changed
}

// applyAuthSettings runs at startup: mail settings, Google OAuth2 and the
// e-mail templates of the users collection. Secrets are never logged.
func applyAuthSettings(app core.App, cfg Config) error {
	s := app.Settings()
	if applyMailSettings(s, cfg.Mail, cfg.PublicURL) {
		if err := app.Save(s); err != nil {
			return err
		}
		if cfg.Mail.Host != "" {
			app.Logger().Info("SMTP configured from env", "host", cfg.Mail.Host, "port", cfg.Mail.Port, "tls", cfg.Mail.TLS)
		}
	}
	if (cfg.Google.ClientID == "") != (cfg.Google.ClientSecret == "") {
		app.Logger().Warn("Google OAuth2 ignored: set both OCC_GOOGLE_CLIENT_ID and OCC_GOOGLE_CLIENT_SECRET")
	}
	users, err := app.FindCollectionByNameOrId(colUsers)
	if err != nil {
		return err
	}
	google := applyGoogleOAuth(users, cfg.Google)
	templates := applyAuthTemplates(users)
	if !google && !templates {
		return nil
	}
	if err := app.Save(users); err != nil {
		return err
	}
	if google {
		app.Logger().Info("Google OAuth2 enabled from env")
	}
	return nil
}

// mailEnabled reports whether e-mails can actually be sent (SMTP configured).
// Without SMTP PocketBase falls back to sendmail, absent from the image.
func mailEnabled(app core.App) bool {
	return app.Settings().SMTP.Enabled
}
