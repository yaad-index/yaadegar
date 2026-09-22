package email

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"strings"
)

// Content is one transactional email as its author thinks about it: a heading,
// some paragraphs, at most one thing to click, and some more paragraphs. It is
// deliberately not HTML — every call site describes what the mail SAYS, and this
// package decides how that is rendered into the two parts a mail carries.
//
// The point is that the rendering decisions live in one place. Before this, each
// sender concatenated its own body string, so there was no shared structure to
// change and no way to add an HTML part without editing every caller (#437).
type Content struct {
	// Title heads the message. Also the sensible default for the subject, though
	// callers set that separately since some subjects differ from the heading.
	Title string
	// Intro paragraphs come before the action, Outro after. Plain sentences: any
	// markup in them is escaped, never interpreted.
	Intro []string
	// Action is the main thing to click, or nil. A mail with no action reads as a
	// notification; one with an action reads as a task.
	Action *Action
	// Secondary is an alternative the reader may take instead, rendered as a plain
	// link rather than a second button. The decay reminder has exactly this shape —
	// keep the reservation, or release it — and two equally weighted buttons would
	// make a routine nudge look like a decision with no default.
	Secondary *Action
	Outro     []string
}

// Paragraphs splits author-written text into paragraphs on blank lines, which is
// how the owner-authored thank-you note is written and the only structure it has.
func Paragraphs(s string) []string {
	var out []string
	for _, p := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Action is a call to action. Label is what the button says; URL is where it goes
// and is also printed in full in the text part, because a text reader cannot click
// a label.
type Action struct {
	Label string
	URL   string
}

// usable reports whether the action can be rendered at all.
//
// ⚠️ The scheme check is load-bearing and was found by a test rather than by
// reading. html/template rewrites a non-http href to "#ZgotmplZ" on its own, so
// the BUTTON was already safe — but this layout prints the same URL a second time
// as pasteable text, and that copy is outside any href context and was being
// emitted verbatim. So the two renderings of one URL disagreed about what was
// safe, and the escaping that made the first one fine said nothing about the
// second. Refusing the action outright keeps every copy consistent.
//
// Every real URL here is built from the instance's configured public link base, so
// anything that is not http(s) means that base is misconfigured. Rendering no
// action is the truthful outcome: there is nothing this mail can ask the reader to
// click.
func (a *Action) usable() bool {
	if a == nil {
		return false
	}
	u := strings.TrimSpace(a.URL)
	lower := strings.ToLower(u)
	return strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://")
}

//go:embed templates/message.html.tmpl
var htmlLayout string

var htmlTemplate = template.Must(template.New("message").Parse(htmlLayout))

// Message renders c into a deliverable message for to, with the given subject.
//
// Both parts are always produced. The text part is not a fallback that nobody
// checks — a good proportion of readers, and every plain-text client, see only it,
// and the confirm flow depends on the link being usable there (#437). So the text
// part spells out the action and its URL rather than assuming a rendered button.
func (c Content) Message(to, subject string) Message {
	return Message{To: to, Subject: subject, Body: c.Text(), HTML: c.HTML()}
}

// Text renders the plain-text part.
func (c Content) Text() string {
	var b strings.Builder
	if t := strings.TrimSpace(c.Title); t != "" {
		b.WriteString(t)
		b.WriteString("\n\n")
	}
	for _, p := range c.Intro {
		if p = strings.TrimSpace(p); p != "" {
			b.WriteString(p)
			b.WriteString("\n\n")
		}
	}
	if c.Action.usable() {
		// Label and URL together. A bare URL on its own line is what the old bodies
		// did, and it tells a reader nothing about what following it will do.
		label := strings.TrimSpace(c.Action.Label)
		if label == "" {
			label = "Open"
		}
		fmt.Fprintf(&b, "%s:\n%s\n\n", label, strings.TrimSpace(c.Action.URL))
	}
	if c.Secondary.usable() {
		label := strings.TrimSpace(c.Secondary.Label)
		if label == "" {
			label = "Or"
		}
		fmt.Fprintf(&b, "%s:\n%s\n\n", label, strings.TrimSpace(c.Secondary.URL))
	}
	for _, p := range c.Outro {
		if p = strings.TrimSpace(p); p != "" {
			b.WriteString(p)
			b.WriteString("\n\n")
		}
	}
	b.WriteString("—\nYaadegar\n")
	return b.String()
}

// HTML renders the HTML part.
//
// Every value interpolated here is user data somewhere — item names, list titles,
// and the owner-authored thank-you note all reach these fields — so the template is
// html/template and nothing bypasses its escaping. The old plain-text bodies made
// this harmless by construction; an HTML part does not, and that is a new hazard
// this change introduces rather than one it inherits.
func (c Content) HTML() string {
	// Drop an unusable action before rendering so the button and the pasteable copy
	// below it cannot disagree; see Action.usable.
	if c.Action != nil && !c.Action.usable() {
		c.Action = nil
	}
	if c.Secondary != nil && !c.Secondary.usable() {
		c.Secondary = nil
	}
	var b bytes.Buffer
	if err := htmlTemplate.Execute(&b, c); err != nil {
		// Execute on a parsed template with plain string fields does not fail in
		// practice; returning empty degrades to a text-only mail rather than
		// sending a half-rendered one.
		return ""
	}
	return b.String()
}
