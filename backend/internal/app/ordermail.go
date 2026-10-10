package app

import (
	"bytes"
	"fmt"
	"html/template"
	"net/mail"
	"strings"
	"sync"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/mailer"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/notify"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

// « Bon de commande » e-mail: when the order is validated (review → paying,
// i.e. the host chose the payer), every member who ordered at least one item
// receives the full order with their own part highlighted at the top and the
// amount to reimburse (or, for the payer, what the others owe them).
//
//   - after-commit hook (OnRecordAfterUpdateSuccess, like the push events);
//   - sent once per party: parties.order_mail_sent_at (hidden, migration
//     1760000020) is claimed by a conditional UPDATE before anything is sent,
//     so double hooks (two saves in one transaction) or a later payer change
//     never resend it;
//   - asynchronous (goroutine): the request never waits for SMTP;
//   - skipped without SMTP, for guests, suspended / deleted accounts,
//     placeholder addresses and users who turned notify_prefs.emails off.
//
// Amounts come from the server summary (buildSummary → domain.BuildSummary)
// and the payments rows; integer cents are formatted at render only.

const colPartiesOrderMailField = "order_mail_sent_at"

// Ember light palette additions for the order e-mail.
const (
	mailElevated  = "#F3F0EA"
	mailBrandSoft = "#FFF3EE"
)

type orderMailer struct {
	app       core.App
	publicURL string
	wg        sync.WaitGroup
}

func newOrderMailer(app core.App, publicURL string) *orderMailer {
	return &orderMailer{app: app, publicURL: publicURL}
}

func (m *orderMailer) bind() {
	m.app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		m.app = se.App
		return se.Next()
	})
	m.app.OnRecordAfterUpdateSuccess(colParties).BindFunc(func(e *core.RecordEvent) error {
		m.onPartyUpdated(e)
		return e.Next()
	})
}

// wait blocks until the queued order e-mails are sent (tests).
func (m *orderMailer) wait() { m.wg.Wait() }

func (m *orderMailer) onPartyUpdated(e *core.RecordEvent) {
	p := e.Record
	if p.Original().GetString("status") != domain.StatusReview {
		return
	}
	// "closed" too: with a single ordering member (the payer) the party is
	// closed in the same transaction, and both hooks see the final record.
	if st := p.GetString("status"); st != domain.StatusPaying && st != domain.StatusClosed {
		return
	}
	if !mailEnabled(m.app) || !m.claim(p.Id) {
		return
	}
	partyID := p.Id
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		sent, err := m.send(partyID)
		if err != nil {
			m.app.Logger().Warn("bon de commande : envoi incomplet", "party", partyID, "sent", sent, "error", err)
			return
		}
		m.app.Logger().Info("bon de commande envoyé", "party", partyID, "recipients", sent)
	}()
}

// claim marks the party as mailed; false when it already was.
func (m *orderMailer) claim(partyID string) bool {
	res, err := m.app.DB().NewQuery(
		"UPDATE " + colParties + " SET " + colPartiesOrderMailField + " = {:now} WHERE id = {:id} AND (" +
			colPartiesOrderMailField + " IS NULL OR " + colPartiesOrderMailField + " = '')").
		Bind(dbx.Params{"now": types.NowDateTime().String(), "id": partyID}).Execute()
	if err != nil {
		m.app.Logger().Warn("bon de commande : marquage impossible", "party", partyID, "error", err)
		return false
	}
	n, _ := res.RowsAffected()
	return n == 1
}

// mailRecipientOK: a real, active, non-guest account that did not opt out.
func mailRecipientOK(u *core.Record) bool {
	email := strings.TrimSpace(u.Email())
	if email == "" || domain.IsPlaceholderEmail(email) {
		return false
	}
	if u.GetBool("banned") || u.GetBool("is_guest") || !u.GetDateTime(fieldDeletedAt).IsZero() {
		return false
	}
	return notify.ParsePrefs([]byte(u.GetString("notify_prefs"))).Emails
}

// send builds and sends one e-mail per eligible ordering member.
func (m *orderMailer) send(partyID string) (int, error) {
	app := m.app
	p, err := app.FindRecordById(colParties, partyID)
	if err != nil {
		return 0, err
	}
	s, rest, err := buildSummary(app, p)
	if err != nil {
		return 0, err
	}
	pays, err := app.FindAllRecords(colPayments, dbx.HashExp{"party": p.Id})
	if err != nil {
		return 0, err
	}
	base := orderMailBase(p, rest, s, pays)

	ids := make([]string, 0, len(s.Participants))
	for _, part := range s.Participants {
		if len(part.Items) > 0 {
			ids = append(ids, part.User.ID)
		}
	}
	users, err := app.FindRecordsByIds(colUsers, ids)
	if err != nil {
		return 0, err
	}
	byID := make(map[string]*core.Record, len(users))
	for _, u := range users {
		byID[u.Id] = u
	}

	appURL := strings.TrimRight(app.Settings().Meta.AppURL, "/")
	if appURL == "" {
		appURL = m.publicURL
	}
	settings := app.Settings()
	from := mail.Address{Name: settings.Meta.SenderName, Address: settings.Meta.SenderAddress}

	sent := 0
	var errs []string
	for _, id := range ids {
		u, ok := byID[id]
		if !ok || !mailRecipientOK(u) {
			continue
		}
		d := base.forRecipient(id, appURL)
		htmlBody, err := renderOrderMailHTML(d)
		if err != nil {
			return sent, err
		}
		msg := &mailer.Message{
			From:    from,
			To:      []mail.Address{{Address: u.Email()}},
			Subject: orderMailSubject(d),
			HTML:    htmlBody,
			Text:    renderOrderMailText(d),
		}
		if err := app.NewMailClient().Send(msg); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		sent++
	}
	if len(errs) > 0 {
		return sent, fmt.Errorf("%d envoi(s) en échec : %s", len(errs), strings.Join(errs, " ; "))
	}
	return sent, nil
}

// ------------------------------------------------------------------- model

type orderMailPerson struct {
	ID          string
	Name        string
	Items       []domain.SummaryItem
	Subtotal    int
	SharedFees  int
	Total       int
	IsRecipient bool
	IsPayer     bool
}

type orderMailData struct {
	AppURL   string
	PartyURL string
	PartyID  string

	Title             string
	Code              string
	Restaurant        string
	RestaurantPhone   string
	RestaurantAddress string
	DeliveryAddress   string
	Provider          string // « Uber Eats », « Téléphone »… (empty if not dispatched)
	SplitEqual        bool

	PayerID   string
	PayerName string
	// Cash: the payer asked to be reimbursed in cash (parties.collect_mode).
	Cash bool

	// recipient
	Me      orderMailPerson
	IsPayer bool
	Owed    int // what the recipient reimburses (non payer)
	// payer variant
	Advanced    int // grand total advanced by the payer
	OwedToPayer int // still owed by the others
	Debtors     int

	People        []orderMailPerson
	ItemsSubtotal int
	DeliveryFee   int
	ServiceFee    int
	Tip           int
	SharedFees    int
	GrandTotal    int

	owedBy map[string]int
}

func orderMailBase(p, rest *core.Record, s domain.Summary, pays []*core.Record) orderMailData {
	d := orderMailData{
		PartyID:         p.Id,
		Title:           strings.TrimSpace(p.GetString("title")),
		Code:            p.GetString("code"),
		DeliveryAddress: strings.TrimSpace(p.GetString("delivery_address")),
		SplitEqual:      s.SplitMode != domain.SplitProportional,
		PayerID:         p.GetString("payer"),
		Cash:            p.GetString("collect_mode") == domain.CollectCash,
		ItemsSubtotal:   s.ItemsSubtotal,
		DeliveryFee:     s.DeliveryFee,
		ServiceFee:      s.ServiceFee,
		Tip:             s.Tip,
		SharedFees:      s.SharedFees,
		GrandTotal:      s.GrandTotal,
		owedBy:          map[string]int{},
	}
	if d.Title == "" {
		d.Title = "Commande " + d.Code
	}
	if rest != nil {
		d.Restaurant = rest.GetString("name")
		if ph := rest.GetString("phone"); ph != "" {
			d.RestaurantPhone = domain.FormatPhone(ph)
		}
		d.RestaurantAddress = rest.GetString("address")
	} else if s.Restaurant != nil {
		d.Restaurant = s.Restaurant.Name
	}
	var dispatch struct {
		Method string `json:"method"`
	}
	if err := p.UnmarshalJSONField("dispatch", &dispatch); err == nil && dispatch.Method != "" {
		if prov, ok := providers.Get(dispatch.Method); ok {
			d.Provider = prov.Name()
		}
	}
	for _, part := range s.Participants {
		if len(part.Items) == 0 {
			continue
		}
		if part.User.ID == d.PayerID {
			d.PayerName = part.User.Name
		}
		d.People = append(d.People, orderMailPerson{
			ID: part.User.ID, Name: part.User.Name, Items: part.Items,
			Subtotal: part.Subtotal, SharedFees: part.SharedFees, Total: part.Total,
			IsPayer: part.User.ID == d.PayerID,
		})
	}
	if d.PayerName == "" {
		for _, part := range s.Participants {
			if part.User.ID == d.PayerID {
				d.PayerName = part.User.Name
			}
		}
	}
	for _, pay := range pays {
		debtor := pay.GetString("debtor")
		if pay.GetString("method") == domain.MethodSelf || debtor == d.PayerID {
			continue
		}
		d.owedBy[debtor] += pay.GetInt("amount")
		if pay.GetString("status") != domain.PaymentConfirmed {
			d.OwedToPayer += pay.GetInt("amount")
			d.Debtors++
		}
	}
	d.Advanced = s.GrandTotal
	return d
}

// forRecipient personalises a copy of the base data.
func (d orderMailData) forRecipient(userID, appURL string) orderMailData {
	out := d
	out.AppURL = appURL
	out.PartyURL = appURL + "/party/" + d.PartyID
	out.People = make([]orderMailPerson, len(d.People))
	for i, p := range d.People {
		p.IsRecipient = p.ID == userID
		out.People[i] = p
		if p.IsRecipient {
			out.Me = p
		}
	}
	out.IsPayer = userID == d.PayerID
	if owed, ok := d.owedBy[userID]; ok {
		out.Owed = owed
	} else if !out.IsPayer {
		out.Owed = out.Me.Total
	}
	return out
}

func orderMailSubject(d orderMailData) string {
	s := "Bon de commande — " + d.Title
	if d.Restaurant != "" {
		s += " (" + d.Restaurant + ")"
	}
	return strings.Join(strings.Fields(s), " ") // no header injection, single line
}

func (d orderMailData) preheader() string {
	if d.IsPayer {
		return "Tu as avancé " + domain.FormatEUR(d.Advanced) + " ; " + d.owedPhrase() + "."
	}
	if d.Cash {
		return "Ta part : " + domain.FormatEUR(d.Owed) + " à rembourser en espèces à " + d.PayerName + "."
	}
	return "Ta part : " + domain.FormatEUR(d.Owed) + " à rembourser à " + d.PayerName + "."
}

// owedPhrase: « 2 collègues te doivent 24,80 € » (payer variant).
func (d orderMailData) owedPhrase() string {
	switch d.Debtors {
	case 0:
		return "personne ne te doit rien"
	case 1:
		return "1 collègue te doit " + domain.FormatEUR(d.OwedToPayer)
	}
	return fmt.Sprintf("%d collègues te doivent %s", d.Debtors, domain.FormatEUR(d.OwedToPayer))
}

// payerLine is the payer's summary sentence in the highlighted block.
func (d orderMailData) payerLine() string {
	if d.Debtors == 0 {
		return "Personne ne te doit rien sur cette commande."
	}
	return d.owedPhrase() + "."
}

// --------------------------------------------------------------------- HTML

// eurHTML formats cents with a non-breaking space (« 12,40 € » never wraps).
func eurHTML(cents int) string { return strings.Replace(domain.FormatEUR(cents), " ", " ", 1) }

var orderMailTmpl = template.Must(template.New("order").Funcs(template.FuncMap{
	"eur": eurHTML,
}).Parse(strings.NewReplacer(
	"[bg]", mailBg, "[surface]", mailSurface, "[border]", mailBorder, "[fg]", mailFg,
	"[muted]", mailMuted, "[brand]", mailBrand, "[brand2]", mailBrand2, "[brandfg]", mailBrandFg,
	"[ink]", mailInk, "[elevated]", mailElevated, "[soft]", mailBrandSoft,
	"[font]", "Inter,Segoe UI,Helvetica,Arial,sans-serif",
).Replace(orderMailHTMLSource)))

// orderMailHTMLSource: same shell as renderMail (mailtemplates.go), tables
// and inline styles only. [tokens] are replaced by the palette at init.
const orderMailHTMLSource = `<div style="display:none;max-height:0;overflow:hidden;opacity:0">{{.Pre}}</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:[bg];padding:24px 0;font-family:[font];color:[fg]">
<tr><td align="center" style="padding:0 12px">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px">
<tr><td style="padding:0 4px 16px;font-size:18px;font-weight:800;letter-spacing:-0.02em"><span style="display:inline-block;width:12px;height:12px;border-radius:6px;background:[brand];background-image:linear-gradient(135deg,[brand],[brand2]);margin-right:8px"></span>OCC Deliveries</td></tr>
<tr><td style="background:[surface];border:1px solid [border];border-radius:20px;padding:28px 24px">
{{with .D}}
<p style="margin:0 0 6px;font-size:13px;line-height:18px;font-weight:700;letter-spacing:0.06em;text-transform:uppercase;color:[ink]">Bon de commande</p>
<h1 style="margin:0 0 6px;font-size:24px;line-height:30px;font-weight:750;letter-spacing:-0.02em;color:[fg]">{{.Title}}</h1>
<p style="margin:0 0 20px;font-size:15px;line-height:22px;color:[muted]">{{if .Restaurant}}{{.Restaurant}} · {{end}}code {{.Code}}{{if .Provider}} · envoyée via {{.Provider}}{{end}}</p>

<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:[soft];border:2px solid [brand];border-radius:16px">
<tr><td style="padding:18px 18px 6px">
<p style="margin:0 0 10px;font-size:13px;line-height:18px;font-weight:700;letter-spacing:0.06em;text-transform:uppercase;color:[ink]">Ta commande</p>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0">
{{range .Me.Items}}<tr>
<td valign="top" width="1%" style="padding:4px 8px 4px 0;font-size:15px;line-height:21px;font-weight:700;color:[fg];white-space:nowrap">{{.Quantity}}&nbsp;×</td>
<td valign="top" style="padding:4px 8px 4px 0;font-size:15px;line-height:21px;color:[fg]">{{.Name}}{{if .OptionsLabel}}<br/><span style="font-size:13px;line-height:18px;color:[muted]">{{.OptionsLabel}}</span>{{end}}{{if .Note}}<br/><em style="font-size:13px;line-height:18px;color:[muted]">« {{.Note}} »</em>{{end}}</td>
<td valign="top" align="right" style="padding:4px 0;font-size:15px;line-height:21px;color:[fg];white-space:nowrap">{{eur .Total}}</td>
</tr>{{end}}
<tr><td colspan="2" style="padding:10px 8px 2px 0;border-top:1px solid [border];font-size:14px;line-height:20px;color:[muted]">Mes plats</td><td align="right" style="padding:10px 0 2px;border-top:1px solid [border];font-size:14px;line-height:20px;color:[muted];white-space:nowrap">{{eur .Me.Subtotal}}</td></tr>
<tr><td colspan="2" style="padding:2px 8px 2px 0;font-size:14px;line-height:20px;color:[muted]">Ma part des frais{{if .SplitEqual}} (partagés à parts égales){{else}} (au prorata){{end}}</td><td align="right" style="padding:2px 0;font-size:14px;line-height:20px;color:[muted];white-space:nowrap">{{eur .Me.SharedFees}}</td></tr>
<tr><td colspan="2" style="padding:2px 8px 12px 0;font-size:15px;line-height:22px;font-weight:700;color:[fg]">Ma part</td><td align="right" style="padding:2px 0 12px;font-size:15px;line-height:22px;font-weight:700;color:[fg];white-space:nowrap">{{eur .Me.Total}}</td></tr>
</table>
</td></tr>
<tr><td style="padding:14px 18px 18px;border-top:1px solid [border]">
{{if .IsPayer}}
<p style="margin:0 0 4px;font-size:14px;line-height:20px;color:[muted]">Tu as avancé</p>
<p style="margin:0 0 6px;font-size:30px;line-height:36px;font-weight:800;letter-spacing:-0.02em;color:[fg]">{{eur .Advanced}}</p>
<p style="margin:0;font-size:15px;line-height:22px;color:[fg]">{{.PayerLine}}</p>
{{else}}
<p style="margin:0 0 4px;font-size:14px;line-height:20px;color:[muted]">Montant à rembourser{{if .Cash}} en espèces{{end}}{{if .PayerName}} à <strong style="color:[fg]">{{.PayerName}}</strong>{{end}}</p>
<p style="margin:0;font-size:30px;line-height:36px;font-weight:800;letter-spacing:-0.02em;color:[fg]">{{eur .Owed}}</p>
{{end}}
</td></tr>
</table>

<table role="presentation" cellpadding="0" cellspacing="0" style="margin:22px 0 8px"><tr><td style="border-radius:14px;background:[brand];background-image:linear-gradient(135deg,[brand],[brand2])">
<a href="{{.PartyURL}}" target="_blank" rel="noopener" style="display:inline-block;padding:14px 26px;font-size:16px;font-weight:700;color:[brandfg];text-decoration:none;border-radius:14px">{{if .IsPayer}}Suivre les remboursements{{else}}Rembourser {{if .PayerName}}{{.PayerName}}{{else}}ma part{{end}}{{end}}</a>
</td></tr></table>
<p style="margin:0 0 24px;font-size:13px;line-height:18px;color:[muted]">{{if .IsPayer}}Confirme chaque remboursement reçu dans l'app.{{else if .Cash}}Remboursement en espèces, de la main à la main : déclare-le dans l'app une fois fait.{{else}}QR code virement (dans ton app bancaire), Revolut, PayPal… : tout est dans l'app, montant prérempli.{{end}}<br/><a href="{{.PartyURL}}" style="color:[ink];word-break:break-all">{{.PartyURL}}</a></p>

<h2 style="margin:0 0 10px;font-size:18px;line-height:24px;font-weight:750;color:[fg]">Commande complète</h2>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border:1px solid [border];border-radius:14px">
{{range .People}}
<tr><td colspan="3" style="padding:12px 14px 4px;background:{{if .IsRecipient}}[soft]{{else}}[elevated]{{end}};font-size:15px;line-height:21px;font-weight:700;color:[fg]">{{.Name}}{{if .IsRecipient}} <span style="font-weight:400;color:[muted]">(toi)</span>{{end}}{{if .IsPayer}} <span style="font-size:12px;font-weight:700;color:[ink]">· a payé</span>{{end}}</td></tr>
{{$rec := .IsRecipient}}{{range .Items}}<tr>
<td valign="top" width="1%" style="padding:4px 6px 4px 14px;font-size:14px;line-height:20px;color:[fg];white-space:nowrap{{if $rec}};background:[soft]{{end}}">{{.Quantity}}&nbsp;×</td>
<td valign="top" style="padding:4px 6px 4px 0;font-size:14px;line-height:20px;color:[fg]{{if $rec}};background:[soft]{{end}}">{{.Name}}{{if .OptionsLabel}} <span style="color:[muted]">— {{.OptionsLabel}}</span>{{end}}{{if .Note}}<br/><em style="font-size:13px;color:[muted]">« {{.Note}} »</em>{{end}}</td>
<td valign="top" align="right" style="padding:4px 14px 4px 0;font-size:14px;line-height:20px;color:[fg];white-space:nowrap{{if $rec}};background:[soft]{{end}}">{{eur .Total}}</td>
</tr>{{end}}
<tr><td colspan="2" style="padding:4px 6px 12px 14px;font-size:13px;line-height:18px;color:[muted];border-bottom:1px solid [border]{{if $rec}};background:[soft]{{end}}">Plats {{eur .Subtotal}} + frais {{eur .SharedFees}}</td><td align="right" style="padding:4px 14px 12px 0;font-size:14px;line-height:20px;font-weight:700;color:[fg];white-space:nowrap;border-bottom:1px solid [border]{{if $rec}};background:[soft]{{end}}">{{eur .Total}}</td></tr>
{{end}}
<tr><td colspan="2" style="padding:12px 6px 2px 14px;font-size:14px;line-height:20px;color:[muted]">Sous-total des plats</td><td align="right" style="padding:12px 14px 2px 0;font-size:14px;line-height:20px;color:[muted];white-space:nowrap">{{eur .ItemsSubtotal}}</td></tr>
{{if .DeliveryFee}}<tr><td colspan="2" style="padding:2px 6px 2px 14px;font-size:14px;line-height:20px;color:[muted]">Livraison</td><td align="right" style="padding:2px 14px 2px 0;font-size:14px;line-height:20px;color:[muted];white-space:nowrap">{{eur .DeliveryFee}}</td></tr>{{end}}
{{if .ServiceFee}}<tr><td colspan="2" style="padding:2px 6px 2px 14px;font-size:14px;line-height:20px;color:[muted]">Frais de service</td><td align="right" style="padding:2px 14px 2px 0;font-size:14px;line-height:20px;color:[muted];white-space:nowrap">{{eur .ServiceFee}}</td></tr>{{end}}
{{if .Tip}}<tr><td colspan="2" style="padding:2px 6px 2px 14px;font-size:14px;line-height:20px;color:[muted]">Pourboire</td><td align="right" style="padding:2px 14px 2px 0;font-size:14px;line-height:20px;color:[muted];white-space:nowrap">{{eur .Tip}}</td></tr>{{end}}
<tr><td colspan="2" style="padding:8px 6px 14px 14px;font-size:16px;line-height:22px;font-weight:800;color:[fg]">Total de la commande</td><td align="right" style="padding:8px 14px 14px 0;font-size:16px;line-height:22px;font-weight:800;color:[fg];white-space:nowrap">{{eur .GrandTotal}}</td></tr>
</table>

{{if or .Restaurant .DeliveryAddress}}
<p style="margin:20px 0 0;padding-top:14px;border-top:1px solid [border];font-size:14px;line-height:21px;color:[muted]">
{{if .Restaurant}}<strong style="color:[fg]">{{.Restaurant}}</strong>{{if .RestaurantPhone}}<br/>Tél. {{.RestaurantPhone}}{{end}}{{if .RestaurantAddress}}<br/>{{.RestaurantAddress}}{{end}}{{end}}
{{if .DeliveryAddress}}{{if .Restaurant}}<br/>{{end}}Livraison : {{.DeliveryAddress}}{{end}}
{{if .PayerName}}<br/>Payé par {{.PayerName}}{{end}}
</p>
{{end}}
{{end}}
</td></tr>
<tr><td style="padding:16px 4px 0;font-size:12px;line-height:16px;color:[muted]">Tu reçois cet e-mail car tu as commandé dans cette party. Tu peux le désactiver dans ton profil (Notifications).<br/>Commandes groupées entre collègues · <a href="{{.D.AppURL}}" style="color:[muted]">{{.D.AppURL}}</a><br/>Développé par OCC MONS Studios</td></tr>
</table></td></tr></table>`

type orderMailView struct {
	orderMailData
	PayerLine string
}

func renderOrderMailHTML(d orderMailData) (string, error) {
	var b bytes.Buffer
	err := orderMailTmpl.Execute(&b, map[string]any{
		"Pre": d.preheader(),
		"D":   orderMailView{orderMailData: d, PayerLine: d.payerLine()},
	})
	return b.String(), err
}

// --------------------------------------------------------------------- text

func renderOrderMailText(d orderMailData) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	eur := domain.FormatEUR
	itemLine := func(it domain.SummaryItem, indent string) {
		s := fmt.Sprintf("%s%d × %s", indent, it.Quantity, it.Name)
		if it.OptionsLabel != "" {
			s += " (" + it.OptionsLabel + ")"
		}
		line("%s — %s", s, eur(it.Total))
		if it.Note != "" {
			line("%s   « %s »", indent, it.Note)
		}
	}

	line("BON DE COMMANDE — %s", d.Title)
	meta := "Code " + d.Code
	if d.Restaurant != "" {
		meta = d.Restaurant + " · " + meta
	}
	if d.Provider != "" {
		meta += " · envoyée via " + d.Provider
	}
	line("%s", meta)
	line("")
	line("== TA COMMANDE ==")
	for _, it := range d.Me.Items {
		itemLine(it, "")
	}
	line("Mes plats : %s", eur(d.Me.Subtotal))
	line("Ma part des frais : %s", eur(d.Me.SharedFees))
	line("Ma part : %s", eur(d.Me.Total))
	line("")
	if d.IsPayer {
		line("Tu as avancé %s. %s", eur(d.Advanced), d.payerLine())
		line("Suivre les remboursements : %s", d.PartyURL)
	} else {
		to := ""
		if d.PayerName != "" {
			to = " à " + d.PayerName
		}
		if d.Cash {
			line("MONTANT À REMBOURSER EN ESPÈCES%s : %s", strings.ToUpper(to), eur(d.Owed))
			line("Déclarer ton remboursement en espèces : %s", d.PartyURL)
		} else {
			line("MONTANT À REMBOURSER%s : %s", strings.ToUpper(to), eur(d.Owed))
			line("Rembourser (QR code virement, Revolut, PayPal… montant prérempli) : %s", d.PartyURL)
		}
	}
	line("")
	line("== COMMANDE COMPLÈTE ==")
	for _, p := range d.People {
		name := p.Name
		if p.IsRecipient {
			name += " (toi)"
		}
		if p.IsPayer {
			name += " · a payé"
		}
		line("%s", name)
		for _, it := range p.Items {
			itemLine(it, "  ")
		}
		line("  Plats %s + frais %s = %s", eur(p.Subtotal), eur(p.SharedFees), eur(p.Total))
	}
	line("")
	line("Sous-total des plats : %s", eur(d.ItemsSubtotal))
	if d.DeliveryFee > 0 {
		line("Livraison : %s", eur(d.DeliveryFee))
	}
	if d.ServiceFee > 0 {
		line("Frais de service : %s", eur(d.ServiceFee))
	}
	if d.Tip > 0 {
		line("Pourboire : %s", eur(d.Tip))
	}
	line("Total de la commande : %s", eur(d.GrandTotal))
	if d.Restaurant != "" {
		line("")
		line("%s", d.Restaurant)
		if d.RestaurantPhone != "" {
			line("Tél. %s", d.RestaurantPhone)
		}
		if d.RestaurantAddress != "" {
			line("%s", d.RestaurantAddress)
		}
	}
	if d.DeliveryAddress != "" {
		line("Livraison : %s", d.DeliveryAddress)
	}
	line("")
	line("--")
	line("OCC Deliveries — commandes groupées entre collègues · %s", d.AppURL)
	line("Tu peux désactiver cet e-mail dans ton profil (Notifications).")
	return b.String()
}
