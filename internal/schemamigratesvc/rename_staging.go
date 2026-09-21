package schemamigratesvc

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schemasvc"
	"github.com/aidanlsb/raven/internal/vault"
)

var frontmatterTypeKeyLine = regexp.MustCompile(`^type\s*:`)

func stageFrontmatterTypeRewrite(raw, newName string) ([]byte, bool) {
	lines := strings.Split(raw, "\n")
	start, end, ok := parser.FrontmatterBounds(lines)
	if !ok || end == -1 {
		return nil, false
	}
	for i := start + 1; i < end; i++ {
		if !frontmatterTypeKeyLine.MatchString(lines[i]) {
			continue
		}
		colon := strings.IndexByte(lines[i], ':')
		if colon < 0 {
			continue
		}
		newLine := "type: " + newName
		if comment := trailingYAMLComment(lines[i][colon+1:]); comment != "" {
			newLine += " " + comment
		}
		lines[i] = newLine
		return []byte(strings.Join(lines, "\n")), true
	}
	return nil, false
}

func trailingYAMLComment(valueSegment string) string {
	if hash := strings.IndexByte(valueSegment, '#'); hash >= 0 {
		return strings.TrimSpace(valueSegment[hash:])
	}
	return ""
}

func (plan *typeRenamePlan) stageReferenceUpdates(vaultPath string, vaultCfg *config.VaultConfig) error {
	idMoves := make(map[string]string, len(plan.DefaultPathPlan.Moves))
	destBySourceRel := make(map[string]string, len(plan.DefaultPathPlan.Moves))
	for _, move := range plan.DefaultPathPlan.Moves {
		idMoves[move.SourceID] = move.DestinationID
		destBySourceRel[move.SourceRelPath] = filepath.Join(vaultPath, move.DestinationRelPath)
	}

	oldIDs := make([]string, 0, len(idMoves))
	for oldID := range idMoves {
		oldIDs = append(oldIDs, oldID)
	}
	sort.SliceStable(oldIDs, func(i, j int) bool {
		return len(oldIDs[i]) > len(oldIDs[j])
	})

	objectRoot := ""
	pageRoot := ""
	if vaultCfg != nil {
		objectRoot = vaultCfg.GetObjectsRoot()
		pageRoot = vaultCfg.GetPagesRoot()
	}

	return vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
		if result.Error != nil {
			return result.Error
		}
		if result.Document == nil {
			return nil
		}

		base := result.Document.RawContent
		if staged, ok := plan.MarkdownFiles[result.Path]; ok {
			base = string(staged)
		}
		updated := base
		for _, oldID := range oldIDs {
			updated = objectsvc.ReplaceAllRefVariants(updated, oldID, oldID, idMoves[oldID], objectRoot, pageRoot)
		}
		if updated == base {
			return nil
		}

		finalPath := result.Path
		if destination, ok := destBySourceRel[result.RelativePath]; ok {
			finalPath = destination
		}
		plan.ReferenceFiles[finalPath] = []byte(updated)
		plan.OptionalChanges = append(plan.OptionalChanges, schemasvc.SchemaChange{
			FilePath:    result.RelativePath,
			ChangeType:  "reference_update",
			Description: fmt.Sprintf("update references after directory move in '%s'", result.RelativePath),
		})
		return nil
	})
}

func looksLikeTemplatePath(value string) bool {
	if value == "" {
		return false
	}
	if strings.Contains(value, "/") || strings.HasSuffix(value, ".md") || strings.HasPrefix(value, "templates") {
		return true
	}
	if strings.Contains(value, "\n") {
		return false
	}
	matched, _ := regexp.MatchString(`^[\w.-]+$`, value)
	return matched && len(value) < 100
}
