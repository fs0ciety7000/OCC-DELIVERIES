package app

import (
	"html"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// French, on-brand e-mail templates of the users collection. Links point to
// the SPA routes (never to the PocketBase admin /_/):
//
//	/auth/verifier/{TOKEN}        e-mail verification
//	/auth/reinitialiser/{TOKEN}   password reset
//	/auth/changer-email/{TOKEN}   e-mail change confirmation
//
// {APP_URL} = settings meta.appURL = OCC_PUBLIC_URL. PocketBase wraps the body
// in its own minimal layout; everything below uses inline styles (e-mail
// clients ignore <style>), with the light « Ember » palette.

const (
	routeVerify      = "/auth/verifier/"
	routeReset       = "/auth/reinitialiser/"
	routeEmailChange = "/auth/changer-email/"
	routeForgot      = "/auth/mot-de-passe-oublie"
)

// Ember light palette (docs/DESIGN_SYSTEM.md) — e-mails cannot use CSS tokens.
const (
	mailBg      = "#FAF8F4"
	mailSurface = "#FFFFFF"
	mailBorder  = "#ECE7DF"
	mailFg      = "#17151A"
	mailMuted   = "#5F5B66"
	mailBrand   = "#FF6A3D"
	mailBrand2  = "#FFB547"
	mailBrandFg = "#1A0B05"
	mailInk     = "#B93A17"
)

type mailContent struct {
	Preheader  string
	Title      string
	Paragraphs []string // trusted HTML (placeholders allowed)
	CTALabel   string
	CTAURL     string
	Footnote   string // trusted HTML
}

// renderMail builds the HTML body of an e-mail. Paragraphs and URLs are
// trusted template strings (they may contain PocketBase placeholders).
func renderMail(c mailContent) string {
	var b strings.Builder
	b.WriteString(`<div style="display:none;max-height:0;overflow:hidden;opacity:0">` + html.EscapeString(c.Preheader) + `</div>`)
	b.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:` + mailBg + `;padding:24px 0;font-family:Inter,Segoe UI,Helvetica,Arial,sans-serif;color:` + mailFg + `">`)
	b.WriteString(`<tr><td align="center" style="padding:0 12px">`)
	b.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:520px">`)
	// brand
	b.WriteString(`<tr><td style="padding:0 4px 16px;font-size:18px;font-weight:800;letter-spacing:-0.02em">`)
	b.WriteString(`<span style="display:inline-block;width:12px;height:12px;border-radius:6px;background:` + mailBrand + `;background-image:linear-gradient(135deg,` + mailBrand + `,` + mailBrand2 + `);margin-right:8px"></span>OCC Deliveries</td></tr>`)
	// card
	b.WriteString(`<tr><td style="background:` + mailSurface + `;border:1px solid ` + mailBorder + `;border-radius:20px;padding:28px 24px">`)
	b.WriteString(`<h1 style="margin:0 0 12px;font-size:24px;line-height:30px;font-weight:750;letter-spacing:-0.02em;color:` + mailFg + `">` + c.Title + `</h1>`)
	for _, p := range c.Paragraphs {
		b.WriteString(`<p style="margin:0 0 14px;font-size:16px;line-height:24px;color:` + mailFg + `">` + p + `</p>`)
	}
	if c.CTAURL != "" {
		b.WriteString(`<table role="presentation" cellpadding="0" cellspacing="0" style="margin:22px 0"><tr><td style="border-radius:14px;background:` + mailBrand + `;background-image:linear-gradient(135deg,` + mailBrand + `,` + mailBrand2 + `)">`)
		b.WriteString(`<a href="` + c.CTAURL + `" target="_blank" rel="noopener" style="display:inline-block;padding:14px 26px;font-size:16px;font-weight:700;color:` + mailBrandFg + `;text-decoration:none;border-radius:14px">` + c.CTALabel + `</a>`)
		b.WriteString(`</td></tr></table>`)
		b.WriteString(`<p style="margin:0 0 14px;font-size:13px;line-height:18px;color:` + mailMuted + `">Le bouton ne marche pas ? Copie ce lien dans ton navigateur :<br/><a href="` + c.CTAURL + `" style="color:` + mailInk + `;word-break:break-all">` + c.CTAURL + `</a></p>`)
	}
	if c.Footnote != "" {
		b.WriteString(`<p style="margin:18px 0 0;padding-top:14px;border-top:1px solid ` + mailBorder + `;font-size:13px;line-height:18px;color:` + mailMuted + `">` + c.Footnote + `</p>`)
	}
	b.WriteString(`</td></tr>`)
	b.WriteString(`<tr><td style="padding:16px 4px 0;font-size:12px;line-height:16px;color:` + mailMuted + `">Commandes groupées entre collègues · <a href="{APP_URL}" style="color:` + mailMuted + `">{APP_URL}</a><br/>Développé par OCC MONS Studios</td></tr>`)
	b.WriteString(`</table></td></tr></table>`)
	return b.String()
}

var verificationTemplate = core.EmailTemplate{
	Subject: "Confirme ton adresse e-mail — {APP_NAME}",
	Body: renderMail(mailContent{
		Preheader:  "Un clic pour confirmer ton adresse et profiter des commandes groupées.",
		Title:      "Bienvenue à table 👋",
		Paragraphs: []string{"Merci d'avoir rejoint {APP_NAME} ! Confirme ton adresse e-mail pour sécuriser ton compte : c'est elle qui te permettra de récupérer ton mot de passe."},
		CTALabel:   "Confirmer mon adresse",
		CTAURL:     "{APP_URL}" + routeVerify + "{TOKEN}",
		Footnote:   "Ce lien est valable 24 heures. Tu n'as pas créé de compte ? Ignore simplement cet e-mail.",
	}),
}

var resetPasswordTemplate = core.EmailTemplate{
	Subject: "Réinitialise ton mot de passe — {APP_NAME}",
	Body: renderMail(mailContent{
		Preheader:  "Choisis un nouveau mot de passe pour ton compte.",
		Title:      "Nouveau mot de passe",
		Paragraphs: []string{"On a reçu une demande pour réinitialiser le mot de passe de ton compte {APP_NAME}. Clique sur le bouton pour en choisir un nouveau."},
		CTALabel:   "Choisir un nouveau mot de passe",
		CTAURL:     "{APP_URL}" + routeReset + "{TOKEN}",
		Footnote:   "Ce lien est valable 30 minutes et ne sert qu'une fois. Tu n'as rien demandé ? Ignore cet e-mail : ton mot de passe actuel reste valable.",
	}),
}

var confirmEmailChangeTemplate = core.EmailTemplate{
	Subject: "Confirme ta nouvelle adresse e-mail — {APP_NAME}",
	Body: renderMail(mailContent{
		Preheader:  "Dernière étape pour changer l'adresse de ton compte.",
		Title:      "Nouvelle adresse e-mail",
		Paragraphs: []string{"Tu as demandé à utiliser cette adresse pour ton compte {APP_NAME}. Confirme le changement avec ton mot de passe actuel."},
		CTALabel:   "Confirmer cette adresse",
		CTAURL:     "{APP_URL}" + routeEmailChange + "{TOKEN}",
		Footnote:   "Ce lien est valable 30 minutes. Tu n'es pas à l'origine de cette demande ? Ignore cet e-mail : rien ne change.",
	}),
}

var authAlertTemplate = core.EmailTemplate{
	Subject: "Nouvelle connexion à ton compte — {APP_NAME}",
	Body: renderMail(mailContent{
		Preheader: "Une connexion depuis un nouvel appareil a eu lieu.",
		Title:     "Nouvelle connexion",
		Paragraphs: []string{
			"Ton compte {APP_NAME} vient d'être utilisé depuis un nouvel appareil ou un nouveau lieu :",
			`<em style="color:` + mailMuted + `">{ALERT_INFO}</em>`,
			"C'était toi ? Tout va bien, rien à faire. Sinon, change ton mot de passe tout de suite : les autres sessions seront déconnectées.",
		},
		CTALabel: "Changer mon mot de passe",
		CTAURL:   "{APP_URL}" + routeForgot,
	}),
}

var otpTemplate = core.EmailTemplate{
	Subject: "Ton code de connexion — {APP_NAME}",
	Body: renderMail(mailContent{
		Preheader:  "Ton code à usage unique.",
		Title:      "Ton code de connexion",
		Paragraphs: []string{`Voici ton code à usage unique : <strong style="font-size:22px;letter-spacing:0.12em">{OTP}</strong>`},
		Footnote:   "Tu n'as rien demandé ? Ignore cet e-mail.",
	}),
}

// testMailBody is the body of the admin « e-mail de test ».
func testMailBody(appURL string) string {
	body := renderMail(mailContent{
		Preheader:  "L'envoi d'e-mails fonctionne.",
		Title:      "Ça marche ! ✅",
		Paragraphs: []string{"Cet e-mail de test confirme que {APP_NAME} peut envoyer des e-mails : vérification d'adresse, mot de passe oublié, changement d'adresse et bons de commande sont opérationnels."},
		Footnote:   "Envoyé depuis l'administration (Utilisateurs → E-mails).",
	})
	r := strings.NewReplacer("{APP_URL}", html.EscapeString(appURL), "{APP_NAME}", "OCC Deliveries")
	return r.Replace(body)
}
