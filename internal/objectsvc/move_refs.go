package objectsvc

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/vault"
)

func UpdateReference(vaultPath string, vaultCfg *config.VaultConfig, sourceID, oldRef, newRef string) error {
	fileSourceID := sourceID
	if idx := strings.Index(sourceID, "#"); idx >= 0 {
		fileSourceID = sourceID[:idx]
	}

	filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, fileSourceID, vaultCfg)
	if err != nil {
		return err
	}
	if err := ValidateContentMutationFilePath(vaultPath, vaultCfg, filePath); err != nil {
		return err
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	oldPattern := "[[" + oldRef + "]]"
	newPattern := "[[" + newRef + "]]"
	newContent := strings.ReplaceAll(string(content), oldPattern, newPattern)

	oldPatternWithText := "[[" + oldRef + "|"
	newPatternWithText := "[[" + newRef + "|"
	newContent = strings.ReplaceAll(newContent, oldPatternWithText, newPatternWithText)

	oldPatternWithFragment := "[[" + oldRef + "#"
	newPatternWithFragment := "[[" + newRef + "#"
	newContent = strings.ReplaceAll(newContent, oldPatternWithFragment, newPatternWithFragment)

	if newContent == string(content) {
		return nil
	}

	return atomicfile.WriteFile(filePath, []byte(newContent), 0o644)
}

func UpdateReferenceAtLine(vaultPath string, vaultCfg *config.VaultConfig, sourceID string, line int, oldRef, newRef string) error {
	if line <= 0 {
		return UpdateReference(vaultPath, vaultCfg, sourceID, oldRef, newRef)
	}

	fileSourceID := sourceID
	if idx := strings.Index(sourceID, "#"); idx >= 0 {
		fileSourceID = sourceID[:idx]
	}

	filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, fileSourceID, vaultCfg)
	if err != nil {
		return err
	}
	if err := ValidateContentMutationFilePath(vaultPath, vaultCfg, filePath); err != nil {
		return err
	}

	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(contentBytes), "\n")
	idx := line - 1
	if idx < 0 || idx >= len(lines) {
		return newError(
			ErrorInvalidInput,
			fmt.Sprintf("line %d is out of range for %s (%d line(s))", line, fileSourceID, len(lines)),
			"Use a valid 1-indexed line number from the source file",
			map[string]interface{}{
				"line":       line,
				"line_count": len(lines),
				"source_id":  fileSourceID,
			},
			nil,
		)
	}

	orig := lines[idx]
	updated := orig

	oldPattern := "[[" + oldRef + "]]"
	newPattern := "[[" + newRef + "]]"
	updated = strings.ReplaceAll(updated, oldPattern, newPattern)

	oldPatternWithText := "[[" + oldRef + "|"
	newPatternWithText := "[[" + newRef + "|"
	updated = strings.ReplaceAll(updated, oldPatternWithText, newPatternWithText)

	oldPatternWithFragment := "[[" + oldRef + "#"
	newPatternWithFragment := "[[" + newRef + "#"
	updated = strings.ReplaceAll(updated, oldPatternWithFragment, newPatternWithFragment)

	if updated == orig {
		return nil
	}
	lines[idx] = updated

	return atomicfile.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0o644)
}

func UpdateAllRefVariantsAtLine(vaultPath string, vaultCfg *config.VaultConfig, sourceID string, line int, oldID, oldBase, newRef, objectRoot, pageRoot string) error {
	fileSourceID := sourceID
	if idx := strings.Index(sourceID, "#"); idx >= 0 {
		fileSourceID = sourceID[:idx]
	}

	filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, fileSourceID, vaultCfg)
	if err != nil {
		return err
	}
	if err := ValidateContentMutationFilePath(vaultPath, vaultCfg, filePath); err != nil {
		return err
	}

	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	updated := ApplyAllRefVariantsAtLine(string(contentBytes), line, oldID, oldBase, newRef, objectRoot, pageRoot)
	if updated == string(contentBytes) {
		return nil
	}

	return atomicfile.WriteFile(filePath, []byte(updated), 0o644)
}

func ReplaceAllRefVariants(content, oldID, oldBase, newRef, objectRoot, pageRoot string) string {
	oldPatterns := make([]string, 0, 6)
	seen := make(map[string]struct{}, 6)
	addPattern := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		oldPatterns = append(oldPatterns, p)
	}

	addPattern(oldID)
	addPattern(oldBase)
	if objectRoot != "" {
		addPattern(objectRoot + oldID)
	}
	if pageRoot != "" && pageRoot != objectRoot {
		addPattern(pageRoot + oldID)
	}

	result := content
	for _, oldPattern := range oldPatterns {
		result = strings.ReplaceAll(result, "[["+oldPattern+"]]", "[["+newRef+"]]")
		result = strings.ReplaceAll(result, "[["+oldPattern+"|", "[["+newRef+"|")
		result = strings.ReplaceAll(result, "[["+oldPattern+"#", "[["+newRef+"#")
	}

	result = replaceFrontmatterBareRefVariants(result, oldPatterns, newRef)
	result = replaceTypeDeclBareRefVariants(result, oldPatterns, newRef)

	return result
}

func ApplyAllRefVariantsAtLine(content string, line int, oldID, oldBase, newRef, objectRoot, pageRoot string) string {
	if line <= 0 {
		return ReplaceAllRefVariants(content, oldID, oldBase, newRef, objectRoot, pageRoot)
	}

	lines := strings.Split(content, "\n")
	idx := line - 1
	if idx < 0 || idx >= len(lines) {
		return content
	}

	orig := lines[idx]
	updated := ReplaceAllRefVariants(orig, oldID, oldBase, newRef, objectRoot, pageRoot)
	if updated == orig {
		return ReplaceAllRefVariants(content, oldID, oldBase, newRef, objectRoot, pageRoot)
	}

	lines[idx] = updated
	return strings.Join(lines, "\n")
}

func replaceFrontmatterBareRefVariants(content string, oldPatterns []string, newRef string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return content
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return content
	}

	frontmatter := strings.Join(lines[1:end], "\n")
	updatedFrontmatter := frontmatter
	for _, oldPattern := range oldPatterns {
		updatedFrontmatter = replaceFrontmatterBareRef(updatedFrontmatter, oldPattern, newRef)
	}
	if updatedFrontmatter == frontmatter {
		return content
	}

	var updatedLines []string
	if updatedFrontmatter != "" {
		updatedLines = strings.Split(updatedFrontmatter, "\n")
	}

	rebuilt := make([]string, 0, 1+len(updatedLines)+len(lines[end:]))
	rebuilt = append(rebuilt, lines[0])
	rebuilt = append(rebuilt, updatedLines...)
	rebuilt = append(rebuilt, lines[end:]...)

	return strings.Join(rebuilt, "\n")
}

func replaceFrontmatterBareRef(frontmatter, oldPattern, newRef string) string {
	oldPattern = strings.TrimSpace(oldPattern)
	if oldPattern == "" || oldPattern == newRef {
		return frontmatter
	}

	escapedOld := regexp.QuoteMeta(oldPattern)
	escapedNew := strings.ReplaceAll(newRef, "$", "$$")
	result := frontmatter

	result = regexp.MustCompile(`(?m)^(\s*[^:\n#]+:\s*)"`+escapedOld+`"(\s*(?:#.*)?)$`).ReplaceAllString(result, `${1}"`+escapedNew+`"${2}`)
	result = regexp.MustCompile(`(?m)^(\s*[^:\n#]+:\s*)'`+escapedOld+`'(\s*(?:#.*)?)$`).ReplaceAllString(result, `${1}'`+escapedNew+`'${2}`)
	result = regexp.MustCompile(`(?m)^(\s*[^:\n#]+:\s*)`+escapedOld+`(\s*(?:#.*)?)$`).ReplaceAllString(result, `${1}`+escapedNew+`${2}`)

	result = regexp.MustCompile(`(?m)^(\s*-\s*)"`+escapedOld+`"(\s*(?:#.*)?)$`).ReplaceAllString(result, `${1}"`+escapedNew+`"${2}`)
	result = regexp.MustCompile(`(?m)^(\s*-\s*)'`+escapedOld+`'(\s*(?:#.*)?)$`).ReplaceAllString(result, `${1}'`+escapedNew+`'${2}`)
	result = regexp.MustCompile(`(?m)^(\s*-\s*)`+escapedOld+`(\s*(?:#.*)?)$`).ReplaceAllString(result, `${1}`+escapedNew+`${2}`)

	result = regexp.MustCompile(`([\[,]\s*)"`+escapedOld+`"(\s*(?:,|\]))`).ReplaceAllString(result, `${1}"`+escapedNew+`"${2}`)
	result = regexp.MustCompile(`([\[,]\s*)'`+escapedOld+`'(\s*(?:,|\]))`).ReplaceAllString(result, `${1}'`+escapedNew+`'${2}`)
	result = regexp.MustCompile(`([\[,]\s*)`+escapedOld+`(\s*(?:,|\]))`).ReplaceAllString(result, `${1}`+escapedNew+`${2}`)

	return result
}

func replaceTypeDeclBareRefVariants(content string, oldPatterns []string, newRef string) string {
	lines := strings.Split(content, "\n")
	changed := false
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "::") {
			continue
		}

		updated := line
		for _, oldPattern := range oldPatterns {
			updated = replaceTypeDeclBareRef(updated, oldPattern, newRef)
		}
		if updated != line {
			lines[i] = updated
			changed = true
		}
	}
	if !changed {
		return content
	}
	return strings.Join(lines, "\n")
}

func replaceTypeDeclBareRef(line, oldPattern, newRef string) string {
	oldPattern = strings.TrimSpace(oldPattern)
	if oldPattern == "" || oldPattern == newRef {
		return line
	}

	escapedOld := regexp.QuoteMeta(oldPattern)
	escapedNew := strings.ReplaceAll(newRef, "$", "$$")
	result := line

	result = regexp.MustCompile(`(=\s*)"`+escapedOld+`"(\s*(?:,|\)))`).ReplaceAllString(result, `${1}"`+escapedNew+`"${2}`)
	result = regexp.MustCompile(`(=\s*)'`+escapedOld+`'(\s*(?:,|\)))`).ReplaceAllString(result, `${1}'`+escapedNew+`'${2}`)
	result = regexp.MustCompile(`(=\s*)`+escapedOld+`(\s*(?:,|\)))`).ReplaceAllString(result, `${1}`+escapedNew+`${2}`)

	result = regexp.MustCompile(`([\[,]\s*)"`+escapedOld+`"(\s*(?:,|\]))`).ReplaceAllString(result, `${1}"`+escapedNew+`"${2}`)
	result = regexp.MustCompile(`([\[,]\s*)'`+escapedOld+`'(\s*(?:,|\]))`).ReplaceAllString(result, `${1}'`+escapedNew+`'${2}`)
	result = regexp.MustCompile(`([\[,]\s*)`+escapedOld+`(\s*(?:,|\]))`).ReplaceAllString(result, `${1}`+escapedNew+`${2}`)

	return result
}

func ChooseReplacementRefBase(oldBase, sourceID, destID string, aliasSlugToID map[string]string, res *resolver.Resolver) string {
	if strings.Contains(oldBase, "/") {
		return destID
	}

	if aliasSlugToID != nil {
		if aliasSlugToID[pages.SlugifyPath(oldBase)] == sourceID {
			return oldBase
		}
	}

	candidate := paths.ShortNameFromID(destID)
	if candidate != "" && res != nil {
		r := res.Resolve(candidate)
		if !r.Ambiguous && r.TargetID == destID {
			return candidate
		}
	}

	return destID
}
