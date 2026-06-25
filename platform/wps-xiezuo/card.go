package wpsxiezuo

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/chenhg5/cc-connect/core"
)

// applyWPSLineBreaks converts engine-emitted "\n" into the form that WPS
// Open Platform markdown actually renders as a line break.
//
// Per WPS docs (https://open.wps.cn/documents/app-integration-dev/guide/robot/webhook
// markdown section), a single "\n" between two non-empty lines is collapsed
// into a space (standard CommonMark behaviour); to force a hard line break
// you must use either "two trailing spaces + \n" or a blank line ("\n\n").
//
// We pick the trailing-spaces form because:
//   - It preserves the original paragraph structure (no spurious blank lines).
//   - It is idempotent if the content already uses "\n\n" — replacing the
//     bare "\n" inside an empty line with "  \n" still renders correctly.
//   - It is safe inside fenced code blocks (the two trailing spaces are
//     whitespace inside a block where the markdown renderer preserves
//     content verbatim, so visually nothing changes).
//
// We deliberately do not normalize "\r\n" → "\n" first; cc-connect engine
// emits Unix newlines, and forcing the transform on already-converted
// "  \n" would over-indent (which is also visually benign but pointless).
func applyWPSLineBreaks(content string) string {
	if content == "" || !strings.Contains(content, "\n") {
		return content
	}
	const marker = "\x00WPS_HARD_BREAK\x00"
	content = strings.ReplaceAll(content, "  \n", marker)
	content = strings.ReplaceAll(content, "\n", "  \n")
	content = strings.ReplaceAll(content, marker, "  \n")
	return content
}

// truncateMarkdown truncates text to fit within limit characters (runes).
// If the text exceeds the limit, it keeps the last wpsCardTruncateKeep characters,
// preferring a paragraph boundary (\n\n) for a clean cut. Falls back to a
// hard cutoff when no paragraph boundary works. Appends a truncation notice.
func truncateMarkdown(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}

	const truncationNotice = "\n\n...（内容过长，已截断）"
	keep := wpsCardTruncateKeep

	paragraphs := strings.Split(text, "\n\n")
	for i := range paragraphs {
		suffix := strings.Join(paragraphs[i:], "\n\n")
		if utf8.RuneCountInString(suffix) <= keep {
			return suffix + truncationNotice
		}
	}

	runes := []rune(text)
	if keep > 0 && keep < len(runes) {
		return string(runes[len(runes)-keep:]) + truncationNotice
	}

	return text + truncationNotice
}

// buildWPSCard constructs a WPS i18n_items card JSON.
// agentName goes in the header subtitle, "CC" goes in the header title.
// Status emoji + label goes in the first element.
// If markdown is non-empty, an hr separator + markdown text element is added.
// --- WPS card structure types ---
// These typed structs replace map[string]any in buildWPSCard for compile-time
// field safety and self-documenting structure. JSON tags match the WPS Open
// Platform v7 card schema exactly.

type wpsCard struct {
	Type    string         `json:"type"`
	Content wpsCardContent `json:"content"`
}

type wpsCardContent struct {
	Card wpsCardInner `json:"card"`
}

type wpsCardInner struct {
	Config    map[string]any `json:"config"`
	I18nItems []wpsI18nItem  `json:"i18n_items"`
}

type wpsI18nItem struct {
	Key   string       `json:"key"`
	Value wpsI18nValue `json:"value"`
}

type wpsI18nValue struct {
	Header   wpsCardHeader    `json:"header"`
	Elements []map[string]any `json:"elements"`
}

type wpsCardHeader struct {
	Title    map[string]any `json:"title"`
	Subtitle map[string]any `json:"subtitle"`
}

func buildWPSCard(agentName string, status core.CardStatus, markdown string) []byte {
	elements := []map[string]any{
		wpsTextElement(fmt.Sprintf("%s %s", statusEmoji(status), statusLabel(status))),
	}

	if markdown != "" {
		markdown = truncateMarkdown(markdown, wpsCardMaxChars)
		markdown = applyWPSLineBreaks(markdown)
		elements = append(elements, wpsHRElement())
		elements = append(elements, wpsTextElement(markdown))
	}

	card := wpsCard{
		Type: "card",
		Content: wpsCardContent{
			Card: wpsCardInner{
				Config: map[string]any{},
				I18nItems: []wpsI18nItem{
					{
						Key: "zh-CN",
						Value: wpsI18nValue{
							Header: wpsCardHeader{
								Title:    wpsPlainElement("CC"),
								Subtitle: wpsPlainElement(agentName),
							},
							Elements: elements,
						},
					},
				},
			},
		},
	}

	b, err := json.Marshal(card)
	if err != nil {
		slog.Error("wps-xiezuo: marshal card JSON", "error", err)
	}
	return b
}

// resolveWPSContent reads the handle's Status (under mutex lock) and builds
// a WPS i18n_items card JSON.
func resolveWPSContent(agentName string, handle *wpsPreviewHandle, content string) []byte {
	handle.mu.Lock()
	status := handle.status
	handle.mu.Unlock()
	return buildWPSCard(agentName, status, content)
}

// wpsTextElement returns a WPS card text element with markdown content type.
func wpsTextElement(content string) map[string]any {
	return map[string]any{
		"text": map[string]any{
			"tag": "text",
			"text": map[string]any{
				"type":    "markdown",
				"content": content,
			},
		},
	}
}

// wpsHRElement returns a WPS card horizontal rule element.
func wpsHRElement() map[string]any {
	return map[string]any{
		"hr": map[string]any{
			"tag": "hr",
		},
	}
}

// wpsPlainElement returns a WPS text element with plain content type.
func wpsPlainElement(content string) map[string]any {
	return map[string]any{
		"tag": "text",
		"text": map[string]any{
			"type":    "plain",
			"content": content,
		},
	}
}
