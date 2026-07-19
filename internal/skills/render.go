package skills

import (
	"fmt"
	"sort"
	"strings"
)

// RenderFiles renders a skill into its installable file set.
func RenderFiles(skill *Skill) (map[string][]byte, error) {
	if skill == nil {
		return nil, fmt.Errorf("skill is nil")
	}

	files := map[string][]byte{
		"SKILL.md": []byte(buildSkillMarkdown(skill)),
	}

	refPaths := make([]string, 0, len(skill.References))
	for p := range skill.References {
		refPaths = append(refPaths, p)
	}
	sort.Strings(refPaths)
	for _, p := range refPaths {
		files[p] = []byte(skill.References[p])
	}

	if strings.TrimSpace(skill.OpenAIMetadata) != "" {
		files["agents/openai.yaml"] = []byte(skill.OpenAIMetadata)
	}

	return files, nil
}

func buildSkillMarkdown(skill *Skill) string {
	entry := strings.TrimSpace(skill.EntryMarkdown)
	if entry == "" {
		entry = "# " + skill.Spec.Title
	}

	return fmt.Sprintf("---\nname: %s\ndescription: %q\n---\n\n%s\n", skill.Spec.ID, skill.Spec.Summary, entry)
}

func sortedRenderedPaths(rendered map[string][]byte) []string {
	paths := make([]string, 0, len(rendered))
	for rel := range rendered {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	return paths
}
