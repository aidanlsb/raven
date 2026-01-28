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

func readEnriched(opts readEnrichedOptions) error {
	// Build rendered content + reference list
	rendered, refs := renderContentWithWikilinkHyperlinks(
		opts.content,
		opts.vaultPath,
		opts.vaultCfg,
	)

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

	fmt.Println(ui.Divider(opts.fileRelPath, width))
	fmt.Println()
	fmt.Print(rendered)
	if !strings.HasSuffix(rendered, "\n") {
		fmt.Println()
	}
	fmt.Println()
	fmt.Println(ui.Divider(fmt.Sprintf("Backlinks (%d)", backlinksCount), width))
	fmt.Println()

	if backlinksCount == 0 {
		fmt.Println(ui.Muted.Render("(none)"))
		return nil
	}

	for i, g := range backlinkGroups {
		if i > 0 {
			fmt.Println()
		}
		fmt.Println(ui.Muted.Render(ui.SymbolAttention) + " " + formatFileLink(g.Source))
		for _, line := range g.Lines {
			fmt.Println(ui.Indent(2, ui.Bullet(line)))
		}
	}

	return nil
}

func renderContentWithWikilinkHyperlinks(content string, vaultPath string, vaultCfg *config.VaultConfig) (string, []readReference) {
	lines := strings.Split(content, "\n")
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

		var b strings.Builder
		last := 0
		for _, m := range matches {
			// Defensive bounds check
			if m.Start < 0 || m.End > len(line) || m.Start < last || m.Start >= m.End {
				continue
			}

			b.WriteString(line[last:m.Start])

			literal := line[m.Start:m.End] // includes [[...]] exactly as written
			refs = append(refs, readReference{Text: m.Target})

			// Attempt to resolve to a file so we can hyperlink it
			path, ok := resolveTargetToRelPath(m.Target, vaultPath, vaultCfg)
			if ok {
				// Update last appended reference to include resolved path
				refs[len(refs)-1].Path = &path

				if shouldEmitHyperlinks() {
					abs := filepath.Join(vaultPath, path)
					url := buildEditorURL(getConfig(), abs, 1)
					b.WriteString(fmt.Sprintf("\x1b]8;;%s\x07%s\x1b]8;;\x07", url, literal))
				} else {
					b.WriteString(literal)
				}
			} else {
				b.WriteString(literal)
			}

			last = m.End
		}
		b.WriteString(line[last:])
		outLines = append(outLines, b.String())
	}

	return strings.Join(outLines, "\n"), refs
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
