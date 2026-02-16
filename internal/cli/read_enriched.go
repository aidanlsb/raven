package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/ui"
	"github.com/aidanlsb/raven/internal/wikilink"
)

type readEnrichedOptions struct {
	vaultPath   string
	vaultCfg    *config.VaultConfig
	reference   string
	objectID    string
	fileAbsPath string
	fileRelPath string
	content     string
	lineCount   int
	start       time.Time
	elapsedMs   int64
}

type readReference struct {
	Text string  `json:"text"`
	Path *string `json:"path,omitempty"`
}

type readBacklinkGroup struct {
	Source string   `json:"source"`
	Lines  []string `json:"lines"`
}

const readRenderMargin = ui.MarkdownRenderMargin

func readEnriched(opts readEnrichedOptions) error {
	// Split content into frontmatter and body
	frontmatter, body := splitFrontmatterBody(opts.content)

	// Pre-process wikilinks in body: convert [[links]] to markdown links
	processedBody, refs := preprocessWikilinks(body, opts.vaultPath, opts.vaultCfg)

	// Fetch backlinks and extract context lines
	backlinkGroups, backlinksCount, err := readBacklinksWithContext(opts.vaultPath, opts.vaultCfg, opts.objectID)
	if err != nil {
		return err
	}

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"path":       opts.fileRelPath,
			"content":    opts.content,
			"line_count": opts.lineCount,
			"references": refs,
			"backlinks":  backlinkGroups,
		}, &Meta{QueryTimeMs: opts.elapsedMs, Count: backlinksCount})
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
	fmt.Println(marginPrefix + ui.DividerWithAccentLabel(fmt.Sprintf("Backlinks (%d)", backlinksCount), width))
	fmt.Println()

	if backlinksCount == 0 {
		fmt.Println(marginPrefix + ui.Muted.Render("(none)"))
		return nil
	}

	for i, g := range backlinkGroups {
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

// preprocessWikilinks collects wikilink reference metadata from the body while
// preserving the original [[wikilink]] text for display.
func preprocessWikilinks(body string, vaultPath string, vaultCfg *config.VaultConfig) (string, []readReference) {
	lines := strings.Split(body, "\n")
	outLines := make([]string, 0, len(lines))
	var refs []readReference

	fs := parser.FenceState{}

	for _, line := range lines {
		// Keep fence marker lines as-is; don't render wikilinks on those lines.
		if fs.UpdateFenceState(line) {
			outLines = append(outLines, line)
			continue
		}
		if fs.InFence {
			outLines = append(outLines, line)
			continue
		}

		sanitized := parser.RemoveInlineCode(line)
		matches := wikilink.FindAllInLine(sanitized, false)
		if len(matches) == 0 {
			outLines = append(outLines, line)
			continue
		}

		for _, m := range matches {
			refs = append(refs, readReference{Text: m.Target})

			// Resolve to file path for JSON metadata/backlinks context.
			path, ok := resolveTargetToRelPath(m.Target, vaultPath, vaultCfg)
			if ok {
				refs[len(refs)-1].Path = &path
			}
		}
		outLines = append(outLines, line)
	}

	return strings.Join(outLines, "\n"), refs
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

func resolveTargetToRelPath(target string, vaultPath string, vaultCfg *config.VaultConfig) (string, bool) {
	res, err := ResolveReference(target, ResolveOptions{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
	})
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(vaultPath, res.FilePath)
	if err != nil {
		return "", false
	}
	return rel, true
}

func readBacklinksWithContext(vaultPath string, vaultCfg *config.VaultConfig, targetObjectID string) ([]readBacklinkGroup, int, error) {
	db, err := index.Open(vaultPath)
	if err != nil {
		return nil, 0, handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
	}
	defer db.Close()

	links, err := db.Backlinks(targetObjectID)
	if err != nil {
		return nil, 0, handleError(ErrDatabaseError, err, "")
	}

	// Group by file_path
	grouped := make(map[string][]model.Reference)
	order := make([]string, 0)
	for _, l := range links {
		if _, exists := grouped[l.FilePath]; !exists {
			order = append(order, l.FilePath)
		}
		grouped[l.FilePath] = append(grouped[l.FilePath], l)
	}

	// Read each file once to extract referenced lines
	fileCache := make(map[string][]string)

	out := make([]readBacklinkGroup, 0, len(order))
	for _, filePath := range order {
		lines, ok := fileCache[filePath]
		if !ok {
			full := filepath.Join(vaultPath, filePath)
			b, readErr := os.ReadFile(full)
			if readErr != nil {
				// If we can't read the file, still include the group but with a placeholder
				out = append(out, readBacklinkGroup{
					Source: filePath,
					Lines:  []string{fmt.Sprintf("(failed to read: %v)", readErr)},
				})
				continue
			}
			lines = strings.Split(string(b), "\n")
			fileCache[filePath] = lines
		}

		var ctx []string
		for _, ref := range grouped[filePath] {
			if ref.Line == nil || *ref.Line <= 0 {
				ctx = append(ctx, "(frontmatter)")
				continue
			}
			idx := *ref.Line - 1
			if idx < 0 || idx >= len(lines) {
				ctx = append(ctx, fmt.Sprintf("(line %d out of range)", *ref.Line))
				continue
			}
			ctx = append(ctx, strings.TrimRight(lines[idx], "\r"))
		}

		ctx = dedupePreserveOrder(ctx)
		out = append(out, readBacklinkGroup{
			Source: filePath,
			Lines:  ctx,
		})
	}

	return out, len(links), nil
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
