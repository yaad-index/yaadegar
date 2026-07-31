package api

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// exportSchemaVersion is the version stamped into the JSON export envelope and
// accepted on import (#26). Bump it only for a breaking format change; an importer
// rejects an envelope whose version it does not recognise.
const exportSchemaVersion = 1

// exportItemsCap bounds a single export. Personal lists are small; this is a sanity
// ceiling, not a paging window.
const exportItemsCap = 2000

// exportItem is the catalog view of an item for backup/portability (#26). It is
// deliberately identity-free and state-free: NONE of reserver identity, reservation
// state, availability, funded amount, ids, or timestamps appears — only the fields
// an owner authored. allow_cobuy / thank_you_template are the RAW per-item overrides
// (null = inherit the list default), so a re-import preserves the owner's inherit
// semantics rather than baking in resolved values.
type exportItem struct {
	Name             string  `json:"name"`
	URL              *string `json:"url,omitempty"`
	ImageURL         *string `json:"image_url,omitempty"`
	PriceAmountMinor *int64  `json:"price_amount_minor,omitempty"`
	PriceCurrency    *string `json:"price_currency,omitempty"`
	Note             *string `json:"note,omitempty"`
	Priority         int     `json:"priority"`
	QuantityWanted   int     `json:"quantity_wanted"`
	AllowCobuy       *bool   `json:"allow_cobuy"`        // null = inherit the list default
	ThankYouTemplate *string `json:"thank_you_template"` // null = inherit the list default
}

// exportEnvelope is the versioned JSON export document — a stable, round-trippable
// contract (documented in the README).
type exportEnvelope struct {
	SchemaVersion int          `json:"schema_version"`
	Items         []exportItem `json:"items"`
}

// exportCSVHeader is the fixed RFC-4180 column order for the CSV export/import.
var exportCSVHeader = []string{
	"name", "url", "image_url", "price_amount_minor", "price_currency",
	"note", "priority", "quantity_wanted", "allow_cobuy", "thank_you_template",
}

func toExportItem(it storage.Item) exportItem {
	e := exportItem{
		Name:             it.Name,
		URL:              it.URL,
		ImageURL:         it.ImageURL,
		Note:             it.Note,
		Priority:         it.Priority,
		QuantityWanted:   it.QuantityWanted,
		AllowCobuy:       it.AllowCobuy,
		ThankYouTemplate: it.ThankYouTemplate,
	}
	if it.Price != nil {
		amt := it.Price.AmountMinor
		cur := it.Price.Currency
		e.PriceAmountMinor = &amt
		e.PriceCurrency = &cur
	}
	return e
}

// handleListExport serves GET /api/v1/lists/{listId}/export?format=json|csv. It is a
// raw handler (registered on the same mux as the generated routes) because file
// download with Content-Disposition is a poor fit for the JSON strict server (#26).
// It runs INSIDE the same middleware chain, so tenant resolution and owner-auth are
// identical to the typed routes; the per-list ownership 403 is enforced here exactly
// as the typed handlers do via ownsList.
func (s *Server) handleListExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "internal error")
		return
	}
	listID := r.PathValue("listId")
	list, err := ts.Lists().Get(ctx, listID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeProblem(w, http.StatusNotFound, "list not found")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "internal error")
		return
	}
	owned, err := s.ownsList(ctx, ts, listID)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !owned {
		writeProblem(w, http.StatusForbidden, "not an owner of this list")
		return
	}

	items, _, err := ts.Items().ListByList(ctx, listID, storage.Page{Limit: exportItemsCap})
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]exportItem, 0, len(items))
	for _, it := range items {
		out = append(out, toExportItem(it))
	}

	switch r.URL.Query().Get("format") {
	case "", "json":
		s.writeExportJSON(w, list.Title, out)
	case "csv":
		s.writeExportCSV(w, list.Title, out)
	default:
		writeProblem(w, http.StatusBadRequest, "unsupported format; use json or csv")
	}
}

func (s *Server) writeExportJSON(w http.ResponseWriter, listTitle string, items []exportItem) {
	body, err := json.MarshalIndent(exportEnvelope{SchemaVersion: exportSchemaVersion, Items: items}, "", "  ")
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", exportDisposition(listTitle, "json"))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		s.logger.Error("export write failed", "err", err)
	}
}

func (s *Server) writeExportCSV(w http.ResponseWriter, listTitle string, items []exportItem) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", exportDisposition(listTitle, "csv"))
	w.WriteHeader(http.StatusOK)
	cw := csv.NewWriter(w)
	if err := cw.Write(exportCSVHeader); err != nil {
		s.logger.Error("export csv header failed", "err", err)
		return
	}
	for _, it := range items {
		if err := cw.Write(exportItemToCSVRow(it)); err != nil {
			s.logger.Error("export csv row failed", "err", err)
			return
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		s.logger.Error("export csv flush failed", "err", err)
	}
}

// exportItemToCSVRow flattens an item to the fixed column order. A nil pointer is an
// empty cell; CSV cannot distinguish null from "" — so on re-import an empty
// allow_cobuy / thank_you_template reads as inherit (the JSON form preserves the
// distinction). This is documented in the README.
func exportItemToCSVRow(it exportItem) []string {
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	amount := ""
	if it.PriceAmountMinor != nil {
		amount = strconv.FormatInt(*it.PriceAmountMinor, 10)
	}
	allowCobuy := ""
	if it.AllowCobuy != nil {
		allowCobuy = strconv.FormatBool(*it.AllowCobuy)
	}
	return []string{
		it.Name, str(it.URL), str(it.ImageURL), amount, str(it.PriceCurrency),
		str(it.Note), strconv.Itoa(it.Priority), strconv.Itoa(it.QuantityWanted),
		allowCobuy, str(it.ThankYouTemplate),
	}
}

// exportDisposition builds a safe attachment filename from the list title: only
// [a-zA-Z0-9-_] survive (others → '-'), so the header can't be injected, with a
// generic fallback when nothing usable remains.
func exportDisposition(listTitle, ext string) string {
	var b strings.Builder
	for _, r := range listTitle {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = "list"
	}
	if len(name) > 64 {
		name = name[:64]
	}
	return `attachment; filename="` + name + "-items." + ext + `"`
}
