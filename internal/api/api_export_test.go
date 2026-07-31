package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #26 Cut 1: owner list export (JSON + CSV), catalog-only — no reserver identity or
// reservation state ever appears.

// exportPath builds the export path for a list.
func exportPath(listID, format string) string {
	p := "/api/v1/lists/" + listID + "/export"
	if format != "" {
		p += "?format=" + format
	}
	return p
}

// A list with a couple of items exports as a versioned JSON envelope carrying only
// the catalog fields.
func TestExport_JSONEnvelopeAndCatalogOnly(t *testing.T) {
	h := newHarness(t)
	list := h.createList("Gifts")
	item := h.pricedItem(*list.Id, 2500, "EUR")
	h.setItemThankYou(t, *item.Id, "thanks {item}")

	resp, body := h.req(http.MethodGet, exportPath(*list.Id, "json"), h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Contains(t, resp.Header.Get("Content-Disposition"), `filename="Gifts-items.json"`)

	var env map[string]any
	require.NoError(t, json.Unmarshal(body, &env))
	assert.EqualValues(t, 1, env["schema_version"])
	items, _ := env["items"].([]any)
	require.Len(t, items, 1)
	row, _ := items[0].(map[string]any)

	// Catalog fields present.
	for _, k := range []string{"name", "price_amount_minor", "price_currency", "priority",
		"quantity_wanted", "allow_cobuy", "thank_you_template"} {
		_, ok := row[k]
		assert.Contains(t, row, k, "catalog field %q must be present", k)
		_ = ok
	}
	assert.Equal(t, "thanks {item}", row["thank_you_template"])

	// Identity/state fields must NEVER appear.
	for _, k := range []string{"id", "list_id", "availability", "reserved_quantity",
		"created_at", "giver_name", "giver_email", "amount_funded"} {
		assert.NotContains(t, row, k, "export must not carry %q", k)
	}
}

// The anonymity guard: reserving an item must NOT change the export — the catalog
// export of a reserved item is byte-identical to its unreserved export.
func TestExport_ReservedItemExportsIdentically(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	item := h.pricedItem(*list.Id, 1000, "EUR")

	_, before := h.req(http.MethodGet, exportPath(*list.Id, "json"), h.ownerHost(), h.ownerToken(), nil)

	// Someone reserves it, leaving a name + email server-side.
	h.reserveGuest(t, *list.ShareSlug, *item.Id, "Secret Giver", "secret@example.com")

	_, after := h.req(http.MethodGet, exportPath(*list.Id, "json"), h.ownerHost(), h.ownerToken(), nil)
	assert.Equal(t, string(before), string(after), "a reservation must not alter the catalog export")
	assert.NotContains(t, string(after), "Secret Giver")
	assert.NotContains(t, string(after), "secret@example.com")
}

// CSV export has the fixed header + one row per item, and is a plain text/csv body.
func TestExport_CSV(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	h.pricedItem(*list.Id, 999, "USD")

	resp, body := h.req(http.MethodGet, exportPath(*list.Id, "csv"), h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/csv")
	assert.Contains(t, resp.Header.Get("Content-Disposition"), `filename="L-items.csv"`)

	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	assert.Equal(t, "name,url,image_url,price_amount_minor,price_currency,note,priority,quantity_wanted,allow_cobuy,thank_you_template",
		strings.TrimSpace(lines[0]))
	assert.Contains(t, lines[1], "999")
	assert.Contains(t, lines[1], "USD")
}

// An unknown format is a 400.
func TestExport_UnsupportedFormat(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	resp, _ := h.req(http.MethodGet, exportPath(*list.Id, "xml"), h.ownerHost(), h.ownerToken(), nil)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// Export is behind owner auth — no bearer token is a 401 (the raw handler is not a
// bypass around the middleware).
func TestExport_RequiresOwnerAuth(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	resp, _ := h.req(http.MethodGet, exportPath(*list.Id, "json"), h.ownerHost(), "", nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// A missing list is a 404.
func TestExport_ListNotFound(t *testing.T) {
	h := newHarness(t)
	resp, _ := h.req(http.MethodGet, exportPath("nope", "json"), h.ownerHost(), h.ownerToken(), nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
