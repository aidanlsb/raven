package objectsvc

import (
	"regexp"
	"strings"

	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/resolver"
)

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
		result = replaceMarkdownLinkDestination(result, oldPattern, newRef)
	}

	result = replaceFrontmatterBareRefVariants(result, oldPatterns, newRef)

	return result
}

func replaceMarkdownLinkDestination(content, oldRef, newRef string) string {
	replacer := strings.NewReplacer(
		"]("+oldRef+")", "]("+newRef+")",
		"]("+oldRef+"#", "]("+newRef+"#",
		"]("+oldRef+"?", "]("+newRef+"?",
		"](<"+oldRef+">)", "](<"+newRef+">)",
		"](<"+oldRef+"#", "](<"+newRef+"#",
		"](<"+oldRef+"?", "](<"+newRef+"?",
	)
	return replacer.Replace(content)
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
