package preview

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetPreviewHeaders_PinsAcceptEncodingToIdentity(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://shop.example/item", nil)
	require.NoError(t, err)

	setPreviewHeaders(req)

	// The header must be present and identity. Leaving it unset is not
	// equivalent: http.Transport would substitute gzip, and a server that sees
	// no Accept-Encoding may compress regardless.
	assert.Equal(t, "identity", req.Header.Get("Accept-Encoding"))
	assert.NotEmpty(t, req.Header.Get("User-Agent"))
	assert.Contains(t, req.Header.Get("Accept"), "text/html")
}

// TestReadBody_DecodesUnsolicitedGzip covers the case that setting the header by
// hand creates: the transport no longer decompresses for us, so a server that
// compresses anyway would hand the parser binary.
func TestReadBody_DecodesUnsolicitedGzip(t *testing.T) {
	const html = `<html><head><title>Real Product</title></head></html>`

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write([]byte(html))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	resp := &http.Response{
		Header: http.Header{"Content-Encoding": []string{"gzip"}},
		Body:   io.NopCloser(bytes.NewReader(buf.Bytes())),
	}

	body, err := readBody(resp)
	require.NoError(t, err)
	assert.Equal(t, html, string(body), "gzip body must arrive decompressed")
}

func TestReadBody_PlainBodyUnchanged(t *testing.T) {
	const html = `<html><head><title>Plain</title></head></html>`
	resp := &http.Response{
		Header: http.Header{},
		Body:   io.NopCloser(strings.NewReader(html)),
	}

	body, err := readBody(resp)
	require.NoError(t, err)
	assert.Equal(t, html, string(body))
}

// TestReadBody_CapsDecompressedSize proves the decompressed side is bounded, so
// a small compressed body cannot expand without limit.
func TestReadBody_CapsDecompressedSize(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write(bytes.Repeat([]byte("a"), maxBodyBytes*4))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.Less(t, buf.Len(), maxBodyBytes, "the compressed input must be small to be a real test")

	resp := &http.Response{
		Header: http.Header{"Content-Encoding": []string{"gzip"}},
		Body:   io.NopCloser(bytes.NewReader(buf.Bytes())),
	}

	body, err := readBody(resp)
	require.NoError(t, err)
	assert.Len(t, body, maxBodyBytes)
}

// TestReadBody_KeepsTruncatedGzipHead documents the deliberate choice to return
// a short read rather than discard it: the cap can cut a compressed stream
// mid-way, and the metadata we want sits in the head of the document.
func TestReadBody_KeepsTruncatedGzipHead(t *testing.T) {
	const html = `<html><head><title>Truncated Product</title></head><body>` +
		`padding padding padding padding padding padding</body></html>`

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write([]byte(html))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	// Chop the tail so the gzip stream cannot be completed.
	truncated := buf.Bytes()[:buf.Len()-12]

	resp := &http.Response{
		Header: http.Header{"Content-Encoding": []string{"gzip"}},
		Body:   io.NopCloser(bytes.NewReader(truncated)),
	}

	body, err := readBody(resp)
	require.NoError(t, err, "a truncated tail must not fail the whole fetch")
	assert.Contains(t, string(body), "<title>Truncated Product</title>",
		"the head of the document must survive truncation")
}

// truncatedReader yields some bytes and then reports the connection died, which
// is what net/http's body reader does when a declared Content-Length is not
// fully delivered.
type truncatedReader struct {
	data []byte
	sent bool
}

func (r *truncatedReader) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		n := copy(p, r.data)
		return n, nil
	}
	return 0, io.ErrUnexpectedEOF
}

// TestReadBody_PlainTruncationIsAnError is the counterpart to the gzip case: on
// an uncompressed body the size cap cannot cause a short read, because
// io.LimitReader signals its limit with a plain io.EOF that io.ReadAll reports
// as success. So an unexpected EOF here is a dropped connection, and returning
// the partial HTML would be a silent success on an incomplete page.
func TestReadBody_PlainTruncationIsAnError(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{},
		Body:   io.NopCloser(&truncatedReader{data: []byte("<html><head><title>Half")}),
	}

	_, err := readBody(resp)
	require.Error(t, err, "a dropped connection must not read as a successful fetch")
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

// TestReadBody_CapAloneIsNotTruncation pins the premise the test above rests on:
// hitting maxBodyBytes on a plain body is a success, not an error.
func TestReadBody_CapAloneIsNotTruncation(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{},
		Body:   io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("a"), maxBodyBytes*2))),
	}

	body, err := readBody(resp)
	require.NoError(t, err, "the size cap is not a failure")
	assert.Len(t, body, maxBodyBytes)
}

func TestReadBody_UnsupportedEncodingIsAnError(t *testing.T) {
	for _, enc := range []string{"br", "deflate", "zstd"} {
		t.Run(enc, func(t *testing.T) {
			resp := &http.Response{
				Header: http.Header{"Content-Encoding": []string{enc}},
				Body:   io.NopCloser(strings.NewReader("\x00\x01binary")),
			}

			_, err := readBody(resp)
			require.Error(t, err, "an encoding we cannot decode must say so")
			assert.Contains(t, err.Error(), enc,
				"the error must name the encoding rather than look like an empty page")
		})
	}
}

func TestReadBody_IdentityHeaderIsPlain(t *testing.T) {
	const html = `<html><head><title>Identity</title></head></html>`
	resp := &http.Response{
		Header: http.Header{"Content-Encoding": []string{"identity"}},
		Body:   io.NopCloser(strings.NewReader(html)),
	}

	body, err := readBody(resp)
	require.NoError(t, err)
	assert.Equal(t, html, string(body))
}

func TestNamesTheSiteItself(t *testing.T) {
	tests := []struct {
		name string
		got  string
		host string
		want bool
	}{
		{"exact host", "Acme.example", "www.acme.example", true},
		{"first label", "Acme", "www.acme.example", true},
		{"case and space insensitive", "  acme.EXAMPLE ", "acme.example", true},
		{"host with port", "shop.example", "shop.example:8443", true},
		{"real product keeps its preview", "Acme Basics Cable", "www.acme.example", false},
		{"substring is not a match", "Acmeium Rainforest Book", "www.acme.example", false},
		{"unrelated name", "Blue Widget 3000", "www.acme.example", false},
		{"empty name", "", "www.acme.example", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, namesTheSiteItself(&tc.got, tc.host))
		})
	}

	assert.False(t, namesTheSiteItself(nil, "www.acme.example"), "a nil name is not a site name")
}
