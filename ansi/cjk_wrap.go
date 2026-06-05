package ansi

import (
	"bytes"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/parser"
)

const cjkNbsp = 0xA0

// WrapCJK wraps text with CJK-aware line breaking.
// It replaces lipgloss.Wrap to properly handle mixed CJK and English text,
// allowing line breaks between any two CJK characters while preserving
// English word boundaries.
func WrapCJK(s string, limit int, breakpoints string) string {
	if limit < 1 {
		return s
	}
	s = wrapCJKaware(s, limit, breakpoints)

	// Preserve ANSI styles across newlines (same as lipgloss.Wrap)
	var buf bytes.Buffer
	w := lipgloss.NewWrapWriter(&buf)
	defer w.Close() //nolint:errcheck
	_, _ = w.Write([]byte(s))
	return buf.String()
}

// wrapCJKaware implements CJK-aware word wrapping.
// Unlike ansi.Wrap which treats all non-space characters as unbreakable words,
// this function allows line breaks between CJK (wide) characters.
func wrapCJKaware(s string, limit int, breakpoints string) string {
	var (
		cluster    string
		buf        bytes.Buffer
		word       bytes.Buffer
		space      bytes.Buffer
		spaceWidth int
		curWidth   int
		wordLen    int
		pstate     = parser.GroundState
	)

	addSpace := func() {
		if spaceWidth == 0 && space.Len() == 0 {
			return
		}
		curWidth += spaceWidth
		buf.Write(space.Bytes())
		space.Reset()
		spaceWidth = 0
	}

	addWord := func() {
		if word.Len() == 0 {
			return
		}
		addSpace()
		curWidth += wordLen
		buf.Write(word.Bytes())
		word.Reset()
		wordLen = 0
	}

	addNewline := func() {
		buf.WriteByte('\n')
		curWidth = 0
		space.Reset()
		spaceWidth = 0
	}

	i := 0
	for i < len(s) {
		state, action := parser.Table.Transition(pstate, s[i])
		if state == parser.Utf8State { //nolint:nestif
			var width int
			cluster, width = ansi.FirstGraphemeCluster(s[i:], ansi.GraphemeWidth)
			i += len(cluster)

			r, _ := utf8.DecodeRuneInString(cluster)
			switch {
			case r != utf8.RuneError && unicode.IsSpace(r) && r != cjkNbsp:
				addWord()
				space.WriteRune(r)
				spaceWidth += width
			case strings.ContainsAny(cluster, breakpoints):
				addSpace()
				if curWidth+wordLen+width > limit {
					word.WriteString(cluster)
					wordLen += width
				} else {
					addWord()
					buf.WriteString(cluster)
					curWidth += width
				}
			default:
				// CJK fix: for wide characters (width > 1), allow breaking
				// at character boundaries. If adding this character would
				// overflow the line, flush the current word first so the
				// break occurs between CJK characters rather than splitting
				// a long CJK sequence across lines incorrectly.
				if width > 1 && wordLen > 0 && curWidth+spaceWidth+wordLen+width > limit {
					addWord()
				}

				if wordLen+width > limit {
					// Hardwrap the word if it's too long
					addWord()
				}

				word.WriteString(cluster)
				wordLen += width

				if curWidth+wordLen+spaceWidth > limit {
					addNewline()
				}

				if wordLen == limit {
					// Hardwrap the word if it's too long
					addWord()
				}
			}

			pstate = parser.GroundState
			continue
		}

		switch action {
		case parser.PrintAction, parser.ExecuteAction:
			switch r := rune(s[i]); {
			case r == '\n':
				if wordLen == 0 {
					if curWidth+spaceWidth > limit {
						curWidth = 0
					} else {
						// preserve whitespaces
						buf.Write(space.Bytes())
					}
					space.Reset()
					spaceWidth = 0
				}

				addWord()
				addNewline()
			case unicode.IsSpace(r):
				addWord()
				space.WriteRune(r)
				spaceWidth++
			case r == '-':
				fallthrough
			case runeContainsAnyCJK(r, breakpoints):
				addSpace()
				if curWidth+wordLen >= limit {
					word.WriteRune(r)
					wordLen++
				} else {
					addWord()
					buf.WriteRune(r)
					curWidth++
				}
			default:
				if curWidth == limit {
					addNewline()
				}

				word.WriteRune(r)
				wordLen++

				if wordLen == limit {
					// Hardwrap the word if it's too long
					addWord()
				}

				if curWidth+wordLen+spaceWidth > limit {
					addNewline()
				}
			}

		default:
			word.WriteByte(s[i])
		}

		// We manage the UTF8 state separately manually above.
		if pstate != parser.Utf8State {
			pstate = state
		}
		i++
	}

	if wordLen == 0 {
		if curWidth+spaceWidth > limit {
			curWidth = 0
		} else {
			// preserve whitespaces
			buf.Write(space.Bytes())
		}
		space.Reset()
		spaceWidth = 0
	}

	addWord()

	return buf.String()
}

func runeContainsAnyCJK(r rune, s string) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}
