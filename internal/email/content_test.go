package email

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #437. Every transactional mail was single-part text/plain assembled by string
// concatenation at its call site, so there was no shared structure to change and
// the confirm mail — the one surface a giver must act on — was a bare line of text
// followed by a raw URL.

func sampleContent() Content {
	return Content{
		Title: "Confirm your reservation",
		Intro: []string{"You reserved Cast iron pan."},
		Action: &Action{
			Label: "Confirm the reservation",
			URL:   "https://lists.example.invalid/confirm?token=synthetic-token",
		},
		Outro: []string{"After that the item is released for someone else to give."},
	}
}

func TestTheTextPartCarriesTheActionAsLabelAndURL(t *testing.T) {
	// The text part is not a fallback nobody checks: a plain-text client sees only
	// it, and the confirm flow is useless if the link is not usable there.
	text := sampleContent().Text()

	assert.Contains(t, text, "Confirm your reservation")
	assert.Contains(t, text, "You reserved Cast iron pan.")
	assert.Contains(t, text, "Confirm the reservation:")
	assert.Contains(t, text, "https://lists.example.invalid/confirm?token=synthetic-token")
	assert.Contains(t, text, "After that the item is released")
}

func TestTheTextPartIsActuallyPlainText(t *testing.T) {
	// The failure this guards is a text part quietly rendered from the HTML one,
	// which reads as tag soup in the client that has nothing else to show.
	text := sampleContent().Text()
	assert.NotContains(t, text, "<")
	assert.NotContains(t, text, "&amp;")
}

func TestTheHTMLPartOffersTheLinkTwice(t *testing.T) {
	// Once as the button, once as selectable text. A client that strips or rewrites
	// the href otherwise leaves the reader with no way to finish what they started.
	html := sampleContent().HTML()
	assert.Contains(t, html, `href="https://lists.example.invalid/confirm?token=synthetic-token"`)
	assert.Contains(t, html, "Or paste this into your browser")
	assert.Equal(t, 2, strings.Count(html, "https://lists.example.invalid/confirm?token=synthetic-token"))
}

func TestTheHTMLPartDependsOnNoRemoteAsset(t *testing.T) {
	// Images are blocked by default in most clients, and a layout that needs one
	// renders broken for exactly the reader who is deciding whether to trust it.
	html := sampleContent().HTML()
	assert.NotContains(t, html, "<img")
	assert.NotContains(t, html, "background-image")
	assert.NotContains(t, html, "<link")
	assert.NotContains(t, html, "@import")
}

// ⚠️ The hazard this change INTRODUCES rather than inherits. Item names, list
// titles and the owner-authored thank-you note are all user data, and they were
// harmless while every body was plain text. In an HTML part they are not.
func TestUserContentIsEscapedInTheHTMLPart(t *testing.T) {
	c := Content{
		Title: `Reserved <script>alert(1)</script>`,
		Intro: []string{`An item called "><img src=x onerror=alert(1)> was reserved.`},
		Outro: []string{`Ampersands & angle brackets < > stay literal.`},
	}
	html := c.HTML()

	// The invariant is that no user data reaches the output as MARKUP. Asserting on
	// a substring like "onerror=" instead would fail on the escaped, inert text that
	// correct behaviour produces — the attribute name survives as literal characters
	// inside a text node, which is exactly what escaping is supposed to do.
	assert.NotContains(t, html, "<script>")
	assert.NotContains(t, html, "<img src=x")
	assert.NotContains(t, html, `"><img`)
	// ...and the text is still there, escaped rather than dropped.
	assert.Contains(t, html, "&lt;script&gt;alert(1)&lt;/script&gt;")
	assert.Contains(t, html, "&lt;img src=x onerror=alert(1)&gt;")
	assert.Contains(t, html, "&amp;")
}

func TestAnActionURLCannotEscapeTheHrefAttribute(t *testing.T) {
	// A link built from a token that is not what we expect must not be able to end
	// the attribute and start another one.
	c := Content{Title: "t", Action: &Action{Label: "Go", URL: `https://e.invalid/x" onmouseover="alert(1)`}}
	html := c.HTML()
	assert.NotContains(t, html, `onmouseover="alert(1)"`)
	assert.NotContains(t, html, `" onmouseover=`)
}

func TestAJavascriptURLIsNeutralisedInEveryCopyOfIt(t *testing.T) {
	// ⚠️ Both copies, which is the whole point. html/template rewrites the href on
	// its own, so asserting only on the button would pass while the layout's second,
	// pasteable rendering of the same URL emitted it verbatim.
	c := Content{Title: "t", Action: &Action{Label: "Go", URL: "javascript:alert(1)"}}
	html := c.HTML()
	assert.NotContains(t, html, "javascript:alert(1)")
	assert.NotContains(t, html, "Or paste this into your browser")
	// The text part has no href context to be saved by, so it must drop it too.
	assert.NotContains(t, c.Text(), "javascript:alert(1)")
}

func TestAnActionIsRenderedOnlyForAWebURL(t *testing.T) {
	for _, u := range []string{"", "   ", "mailto:a@example.invalid", "file:///etc/passwd", "//example.invalid/x"} {
		c := Content{Title: "t", Action: &Action{Label: "Go", URL: u}}
		assert.NotContains(t, c.HTML(), "Or paste this into your browser", "URL %q", u)
		assert.NotContains(t, c.Text(), "Go:", "URL %q", u)
	}
	for _, u := range []string{"https://e.invalid/x", "http://e.invalid/x", "HTTPS://e.invalid/x"} {
		c := Content{Title: "t", Action: &Action{Label: "Go", URL: u}}
		assert.Contains(t, c.HTML(), "Or paste this into your browser", "URL %q", u)
		assert.Contains(t, c.Text(), "Go:", "URL %q", u)
	}
}

func TestContentWithNoActionRendersNoButton(t *testing.T) {
	// A notification and a task should not look the same. Several senders — the
	// thank-you note, the co-buying confirmation — have nothing to click.
	c := Content{Title: "Thank you", Intro: []string{"A note from the list owner."}}
	html := c.HTML()
	text := c.Text()

	assert.NotContains(t, html, "Or paste this into your browser")
	assert.NotContains(t, html, "<a href")
	assert.NotContains(t, text, "Open:")
	assert.Contains(t, text, "A note from the list owner.")
}

func TestMessageCarriesBothParts(t *testing.T) {
	m := sampleContent().Message("giver@example.invalid", "Confirm your reservation")

	assert.Equal(t, "giver@example.invalid", m.To)
	assert.Equal(t, "Confirm your reservation", m.Subject)
	require.NotEmpty(t, m.Body, "the text part is never optional")
	require.NotEmpty(t, m.HTML)
	assert.Contains(t, m.Body, "https://lists.example.invalid/confirm?token=synthetic-token")
	assert.Contains(t, m.HTML, "https://lists.example.invalid/confirm?token=synthetic-token")
}

func TestAnEmptyParagraphIsDroppedRatherThanRenderedBlank(t *testing.T) {
	// Call sites build paragraph slices conditionally, so an empty string is a
	// normal input rather than a mistake.
	c := Content{Title: "T", Intro: []string{"", "   ", "Real."}}
	assert.Contains(t, c.Text(), "Real.")
	assert.NotContains(t, c.Text(), "\n\n\n\n")
}
