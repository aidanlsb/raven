package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/ui"
)

type readEnrichedOptions struct {
	fileRelPath    string
	content        string
	lineCount      int
	elapsedMs      int64
	references     []readsvc.ReadReference
	backlinks      []readsvc.ReadBacklinkGroup
	backlinksCount int
}

const readRenderMargin = ui.MarkdownRenderMargin

func readEnriched(opts readEnrichedOptions) error {
	// Split content into frontmatter and body
	frontmatter, body := splitFrontmatterBody(opts.content)

	processedBody := body

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"path":       opts.fileRelPath,
			"content":    opts.content,
			"line_count": opts.lineCount,
			"references": opts.references,
			"backlinks":  opts.backlinks,
		}, &Meta{QueryTimeMs: opts.elapsedMs, Count: opts.backlinksCount})
		return nil
	}

	display := ui.NewDisplayContext()
	width := display.TermWidth
	if width <= 0 {
		width = ui.DefaultTermWidth
	}
	margin := 0
	if display.IsTTY {
		margin = readRenderMargin
	}
	marginPrefix := strings.Repeat(" ", margin)

	// Render body through glamour when outputting to a TTY
	renderedBody := processedBody
	if display.IsTTY {
		if rendered, renderErr := ui.RenderMarkdown(processedBody, width); renderErr == nil {
			renderedBody = rendered
		}
		renderedBody = renderTraitsStyled(renderedBody)
	}

	fmt.Println(marginPrefix + ui.DividerWithAccentLabel(opts.fileRelPath, width))
	fmt.Println()

	// Print frontmatter as-is (raw YAML)
	if frontmatter != "" {
		renderedFrontmatter := frontmatter
		if display.IsTTY {
			renderedFrontmatter = ui.Muted.Render(frontmatter)
		}
		renderedFrontmatter = indentBlock(renderedFrontmatter, margin)
		fmt.Print(renderedFrontmatter)
		if !strings.HasSuffix(renderedFrontmatter, "\n") {
			fmt.Println()
		}
	}

	// Print rendered body
	fmt.Print(renderedBody)
	if !strings.HasSuffix(renderedBody, "\n") {
		fmt.Println()
	}

	fmt.Println()
	fmt.Println(marginPrefix + ui.DividerWithAccentLabel(fmt.Sprintf("Backlinks (%d)", opts.backlinksCount), width))
	fmt.Println()

	if opts.backlinksCount == 0 {
		fmt.Println(marginPrefix + ui.Muted.Render("(none)"))
		return nil
	}

	for i, g := range opts.backlinks {
		if i > 0 {
			fmt.Println()
		}
		fmt.Println(marginPrefix + ui.Muted.Render(ui.SymbolAttention) + " " + formatFileLink(g.Source))
		for _, line := range g.Lines {
			fmt.Println(marginPrefix + ui.Indent(2, ui.Bullet(line)))
		}
	}

	return nil
}

// splitFrontmatterBody splits file content into the raw frontmatter block
// (including --- delimiters) and the body that follows it.
func splitFrontmatterBody(content string) (frontmatter string, body string) {
	lines := strings.Split(content, "\n")
	_, endLine, ok := parser.FrontmatterBounds(lines)
	if !ok || endLine == -1 {
		return "", content
	}
	// Include everything up to and including the closing ---
	frontmatter = strings.Join(lines[:endLine+1], "\n") + "\n"
	// Body starts after the closing ---
	if endLine+1 < len(lines) {
		body = strings.Join(lines[endLine+1:], "\n")
	}
	return frontmatter, body
}

func renderTraitsStyled(content string) string {
	return ui.HighlightTraits(content)
}

func indentBlock(content string, spaces int) string {
	if spaces <= 0 || content == "" {
		return content
	}
	prefix := strings.Repeat(" ", spaces)
	parts := strings.SplitAfter(content, "\n")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = prefix + part
	}
	return strings.Join(parts, "")
}

func formatFileLink(relPath string) string {
	if !shouldEmitHyperlinks() {
		return ui.FilePath(relPath)
	}
	vaultPath := getVaultPath()
	if vaultPath == "" {
		return ui.FilePath(relPath)
	}
	abs := filepath.Join(vaultPath, relPath)
	url := buildEditorURL(getConfig(), abs, 1)
	return ui.FilePath(fmt.Sprintf("\x1b]8;;%s\x07%s\x1b]8;;\x07", url, relPath))
}
