package api

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// maxImportItems caps a single import; maxImportBytes bounds the request body. Both
// are DoS guards — personal lists are small (#26).
const (
	maxImportItems = 500
	maxImportBytes = 1 << 20 // 1 MiB
)

// importError is one row's rejection, reported so the owner can fix and re-upload.
// row is 1-based: the item index for JSON, the file line number for CSV.
type importError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

// handleListImport serves POST /api/v1/lists/{listId}/import, creating items on the
// target list from an uploaded JSON envelope or CSV (#26 Cut 2). Raw handler, same
// mux + middleware as export, so tenant + owner auth (and the per-list 403) are
// identical. Atomic: it validates EVERY row first and, on any invalid row, returns
// 400 with the full per-row error list and creates nothing; a fully-valid batch is
// then created in a single DB transaction (CreateMany), so a mid-batch DB error also
// leaves the list untouched.
func (s *Server) handleListImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "internal error")
		return
	}
	listID := r.PathValue("listId")
	if _, err := ts.Lists().Get(ctx, listID); err != nil {
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

	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeProblem(w, http.StatusRequestEntityTooLarge, "import file is too large")
		return
	}

	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	var items []exportItem
	switch mediaType {
	case "application/json":
		items, err = parseImportJSON(body)
	case "text/csv":
		items, err = parseImportCSV(body)
	default:
		writeProblem(w, http.StatusUnsupportedMediaType,
			"import requires Content-Type application/json or text/csv")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(items) > maxImportItems {
		writeProblem(w, http.StatusBadRequest,
			"too many items; the import limit is "+strconv.Itoa(maxImportItems))
		return
	}

	// Validate every row first. Any failure aborts the whole import (create nothing).
	toCreate := make([]storage.Item, 0, len(items))
	var rowErrs []importError
	for i, it := range items {
		si, verr := validateImportItem(it, listID)
		if verr != "" {
			rowErrs = append(rowErrs, importError{Row: i + 1, Message: verr})
			continue
		}
		toCreate = append(toCreate, si)
	}
	if len(rowErrs) > 0 {
		writeImportErrors(w, rowErrs)
		return
	}

	created, err := ts.Items().CreateMany(ctx, toCreate)
	if err != nil {
		s.logger.ErrorContext(ctx, "import create failed", "list_id", listID, "error", err)
		writeProblem(w, http.StatusInternalServerError, "could not import the items")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]int{"created": len(created)})
}

// parseImportJSON accepts the export envelope verbatim and enforces a known
// schema_version so a future v2 export can't silently mis-import (#26).
func parseImportJSON(body []byte) ([]exportItem, error) {
	var env exportEnvelope
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&env); err != nil {
		return nil, errors.New("invalid JSON import: " + err.Error())
	}
	if env.SchemaVersion != exportSchemaVersion {
		return nil, errors.New("unsupported schema_version; this server accepts version " +
			strconv.Itoa(exportSchemaVersion))
	}
	return env.Items, nil
}

// parseImportCSV parses a CSV whose header matches the export column order. An empty
// cell is a nil field — so CSV cannot express an explicit "" (opt-out) and an empty
// allow_cobuy / thank_you_template reads as inherit (documented).
func parseImportCSV(body []byte) ([]exportItem, error) {
	rd := csv.NewReader(strings.NewReader(string(body)))
	rows, err := rd.ReadAll()
	if err != nil {
		return nil, errors.New("invalid CSV import: " + err.Error())
	}
	if len(rows) == 0 {
		return nil, errors.New("CSV import is empty")
	}
	if !slicesEqual(rows[0], exportCSVHeader) {
		return nil, errors.New("CSV header must be: " + strings.Join(exportCSVHeader, ","))
	}
	out := make([]exportItem, 0, len(rows)-1)
	for _, row := range rows[1:] {
		it, cerr := csvRowToImportItem(row)
		if cerr != nil {
			return nil, cerr
		}
		out = append(out, it)
	}
	return out, nil
}

func csvRowToImportItem(row []string) (exportItem, error) {
	if len(row) != len(exportCSVHeader) {
		return exportItem{}, errors.New("CSV row has the wrong number of columns")
	}
	strPtr := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	it := exportItem{
		Name:             row[0],
		URL:              strPtr(row[1]),
		ImageURL:         strPtr(row[2]),
		PriceCurrency:    strPtr(row[4]),
		Note:             strPtr(row[5]),
		ThankYouTemplate: strPtr(row[9]),
	}
	if row[3] != "" {
		amt, err := strconv.ParseInt(row[3], 10, 64)
		if err != nil {
			return exportItem{}, errors.New("price_amount_minor is not a number: " + row[3])
		}
		it.PriceAmountMinor = &amt
	}
	if row[6] != "" {
		p, err := strconv.Atoi(row[6])
		if err != nil {
			return exportItem{}, errors.New("priority is not a number: " + row[6])
		}
		it.Priority = p
	}
	if row[7] != "" {
		q, err := strconv.Atoi(row[7])
		if err != nil {
			return exportItem{}, errors.New("quantity_wanted is not a number: " + row[7])
		}
		it.QuantityWanted = q
	}
	if row[8] != "" {
		b, err := strconv.ParseBool(row[8])
		if err != nil {
			return exportItem{}, errors.New("allow_cobuy must be true, false, or empty: " + row[8])
		}
		it.AllowCobuy = &b
	}
	return it, nil
}

// validateImportItem checks one row and maps it to a storage.Item bound to listID.
// It returns a non-empty message on the first problem.
func validateImportItem(it exportItem, listID string) (storage.Item, string) {
	if strings.TrimSpace(it.Name) == "" {
		return storage.Item{}, "name is required"
	}
	if (it.PriceAmountMinor == nil) != (it.PriceCurrency == nil) {
		return storage.Item{}, "price_amount_minor and price_currency must be set together"
	}
	var price *storage.Money
	if it.PriceAmountMinor != nil {
		if *it.PriceAmountMinor <= 0 {
			return storage.Item{}, "price_amount_minor must be positive"
		}
		price = &storage.Money{AmountMinor: *it.PriceAmountMinor, Currency: *it.PriceCurrency}
	}
	if it.Priority < 0 {
		return storage.Item{}, "priority must be zero or greater"
	}
	return storage.Item{
		ListID:           listID,
		Name:             it.Name,
		URL:              it.URL,
		ImageURL:         it.ImageURL,
		Price:            price,
		Note:             it.Note,
		Priority:         it.Priority,
		QuantityWanted:   it.QuantityWanted, // <1 is normalised to 1 by the storage prep
		AllowCobuy:       it.AllowCobuy,
		ThankYouTemplate: it.ThankYouTemplate,
	}, ""
}

func writeImportErrors(w http.ResponseWriter, errs []importError) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "about:blank",
		"title":  "Import validation failed",
		"status": http.StatusBadRequest,
		"detail": "some rows are invalid; nothing was imported",
		"errors": errs,
	})
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
