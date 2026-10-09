package app

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

func (h *handlers) export(e *core.RequestEvent) error {
	party, err := partyForMember(e)
	if err != nil {
		return err
	}
	format := strings.ToLower(e.Request.URL.Query().Get("format"))
	if format == "" {
		format = "txt"
	}

	s, rest, err := buildSummary(e.App, party)
	if err != nil {
		return err
	}

	var (
		data        []byte
		contentType string
	)
	switch format {
	case "json":
		data, err = json.MarshalIndent(s, "", "  ")
		if err != nil {
			return err
		}
		contentType = "application/json; charset=utf-8"
	case "csv":
		data, err = exportCSV(s)
		if err != nil {
			return err
		}
		contentType = "text/csv; charset=utf-8"
	case "txt":
		data = exportTXT(party, rest, s)
		contentType = "text/plain; charset=utf-8"
	default:
		return badRequest("Format d'export inconnu (csv, txt ou json).")
	}

	filename := fmt.Sprintf("occ-%s.%s", strings.ToLower(party.GetString("code")), format)
	e.Response.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	return e.Blob(http.StatusOK, contentType, data)
}

func exportCSV(s domain.Summary) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\xEF\xBB\xBF") // BOM so that spreadsheet apps read UTF-8
	w := csv.NewWriter(&buf)
	w.Comma = ';'
	rows := [][]string{{"Participant", "Article", "Options", "Remarque", "Quantité", "Prix unitaire (EUR)", "Total (EUR)"}}
	for _, p := range s.Participants {
		for _, it := range p.Items {
			rows = append(rows, []string{p.User.Name, it.Name, it.OptionsLabel, it.Note,
				fmt.Sprint(it.Quantity), domain.FormatAmount(it.UnitPrice), domain.FormatAmount(it.Total)})
		}
		if len(p.Items) > 0 {
			rows = append(rows, []string{p.User.Name, "Frais partagés", "", "", "", "", domain.FormatAmount(p.SharedFees)})
		}
	}
	rows = append(rows, []string{"TOTAL", "", "", "", "", "", domain.FormatAmount(s.GrandTotal)})
	if err := w.WriteAll(rows); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func exportTXT(party, rest *core.Record, s domain.Summary) []byte {
	var b strings.Builder
	if title := party.GetString("title"); title != "" {
		fmt.Fprintf(&b, "%s (code %s)\n\n", title, party.GetString("code"))
	} else {
		fmt.Fprintf(&b, "Commande %s\n\n", party.GetString("code"))
	}
	b.WriteString(providers.CartText(providerRestaurant(rest), s))
	b.WriteString("\n\nParts par participant\n")
	b.WriteString(strings.Repeat("-", 32) + "\n")
	for _, p := range s.Participants {
		if len(p.Items) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s — %s (articles %s + frais %s)\n", p.User.Name,
			domain.FormatEUR(p.Total), domain.FormatEUR(p.Subtotal), domain.FormatEUR(p.SharedFees))
		for _, it := range p.Items {
			line := fmt.Sprintf("   %d × %s", it.Quantity, it.Name)
			if it.OptionsLabel != "" {
				line += " (" + it.OptionsLabel + ")"
			}
			if it.Note != "" {
				line += " — « " + it.Note + " »"
			}
			b.WriteString(line + "\n")
		}
	}
	return []byte(b.String())
}
