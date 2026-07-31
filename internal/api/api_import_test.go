package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

func importPath(listID string) string { return "/api/v1/lists/" + listID + "/import" }

// importRaw posts a raw body with an explicit Content-Type (for CSV and exact-bytes
// round-trip; the JSON map path uses h.req).
func (h *harness) importRaw(listID, contentType, body, token string) (*http.Response, []byte) {
	h.t.Helper()
	req, err := http.NewRequest(http.MethodPost, "http://"+h.ownerHost()+importPath(listID), strings.NewReader(body))
	require.NoError(h.t, err)
	req.Host = h.ownerHost()
	req.Header.Set("Content-Type", contentType)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := &responseRecorder{header: http.Header{}, body: &bytes.Buffer{}}
	h.h.ServeHTTP(rec, req)
	return rec.result(), rec.body.Bytes()
}

// listItemNames reads the target list's item names (owner view).
func (h *harness) listItemNames(t *testing.T, listID string) []string {
	t.Helper()
	resp, body := h.req(http.MethodGet, "/api/v1/lists/"+listID+"/items", h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	page := decode[gen.ItemPage](t, body)
	var names []string
	for _, it := range *page.Items {
		names = append(names, *it.Name)
	}
	return names
}

// A JSON envelope creates items on the target list.
func TestImport_JSONCreatesItems(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	env := map[string]any{
		"schema_version": 1,
		"items": []map[string]any{
			{"name": "Book", "priority": 0, "quantity_wanted": 1},
			{"name": "Mug", "price_amount_minor": 1500, "price_currency": "EUR", "quantity_wanted": 2},
		},
	}
	resp, body := h.req(http.MethodPost, importPath(*list.Id), h.ownerHost(), h.ownerToken(), env)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	assert.JSONEq(t, `{"created":2}`, string(body))
	assert.ElementsMatch(t, []string{"Book", "Mug"}, h.listItemNames(t, *list.Id))
}

// A CSV upload creates items.
func TestImport_CSVCreatesItems(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	csv := "name,url,image_url,price_amount_minor,price_currency,note,priority,quantity_wanted,allow_cobuy,thank_you_template\n" +
		"Lamp,,,2500,USD,,0,1,,\n" +
		"Chair,,,,,nice,1,3,false,thanks {item}\n"
	resp, body := h.importRaw(*list.Id, "text/csv", csv, h.ownerToken())
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	assert.ElementsMatch(t, []string{"Lamp", "Chair"}, h.listItemNames(t, *list.Id))
}

// Export then import the exact bytes into a fresh list — the catalog round-trips.
func TestImport_RoundTripsExport(t *testing.T) {
	h := newHarness(t)
	src := h.createList("Source")
	item := h.pricedItem(*src.Id, 4200, "GBP")
	h.setItemThankYou(t, *item.Id, "cheers {item}")

	_, exported := h.req(http.MethodGet, exportPath(*src.Id, "json"), h.ownerHost(), h.ownerToken(), nil)

	dst := h.createList("Dest")
	resp, body := h.importRaw(*dst.Id, "application/json", string(exported), h.ownerToken())
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)

	// The destination now re-exports to the same catalog as the source.
	_, srcAgain := h.req(http.MethodGet, exportPath(*src.Id, "json"), h.ownerHost(), h.ownerToken(), nil)
	_, dstExport := h.req(http.MethodGet, exportPath(*dst.Id, "json"), h.ownerHost(), h.ownerToken(), nil)
	assert.Equal(t, string(srcAgain), string(dstExport), "round-tripped catalog matches the source")
}

// Any invalid row aborts the whole import: 400 with per-row errors, create nothing.
func TestImport_InvalidRowsAreAtomic(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	env := map[string]any{
		"schema_version": 1,
		"items": []map[string]any{
			{"name": "Good", "quantity_wanted": 1},
			{"name": "", "quantity_wanted": 1},                                      // bad: no name
			{"name": "NoCurrency", "price_amount_minor": 100, "quantity_wanted": 1}, // bad: price half-set
		},
	}
	resp, body := h.req(http.MethodPost, importPath(*list.Id), h.ownerHost(), h.ownerToken(), env)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var problem struct {
		Errors []struct {
			Row     int    `json:"row"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(body, &problem))
	require.Len(t, problem.Errors, 2)
	assert.Equal(t, 2, problem.Errors[0].Row)
	assert.Equal(t, 3, problem.Errors[1].Row)

	// Nothing was created — not even the valid first row.
	assert.Empty(t, h.listItemNames(t, *list.Id))
}

// An unknown schema_version is rejected clearly (no silent mis-import).
func TestImport_UnknownSchemaVersion(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	env := map[string]any{"schema_version": 2, "items": []map[string]any{{"name": "X"}}}
	resp, _ := h.req(http.MethodPost, importPath(*list.Id), h.ownerHost(), h.ownerToken(), env)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Empty(t, h.listItemNames(t, *list.Id))
}

// A non-JSON, non-CSV content type is a 415.
func TestImport_UnsupportedContentType(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	resp, _ := h.importRaw(*list.Id, "text/plain", "whatever", h.ownerToken())
	assert.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode)
}

// Cross-owner: an authenticated owner exporting or importing a DIFFERENT owner's
// list in the same tenant gets 403 — identical to the typed routes' ownership guard.
func TestImportExport_CrossOwner403(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	ts := h.store.ForTenant(h.tenant)
	other, err := ts.Users().Create(ctx, storage.User{Name: "Other"})
	require.NoError(t, err)
	othersList, err := ts.Lists().Create(ctx, storage.List{Title: "Theirs"}, other.ID)
	require.NoError(t, err)

	// h.ownerToken() authenticates the seeded owner, who does NOT own othersList.
	resp, _ := h.req(http.MethodGet, exportPath(othersList.ID, "json"), h.ownerHost(), h.ownerToken(), nil)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "export another owner's list → 403")

	env := map[string]any{"schema_version": 1, "items": []map[string]any{{"name": "X"}}}
	resp, _ = h.req(http.MethodPost, importPath(othersList.ID), h.ownerHost(), h.ownerToken(), env)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "import into another owner's list → 403")
}
