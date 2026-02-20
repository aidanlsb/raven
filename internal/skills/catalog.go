package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed library/*/skill.yaml library/*/body.md library/*/references/*.md library/*/agents/openai.yaml
var libraryFS embed.FS

// LoadCatalog loads all embedded Raven skills.
func LoadCatalog() (map[string]*Skill, error) {
	entries, err := fs.ReadDir(libraryFS, "library")
	if err != nil {
		return nil, fmt.Errorf("read embedded skill library: %w", err)
	}

	catalog := make(map[string]*Skill)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		base := path.Join("library", dirName)

		specData, err := fs.ReadFile(libraryFS, path.Join(base, "skill.yaml"))
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", dirName, err)
		}

		var spec Spec
		if err := yaml.Unmarshal(specData, &spec); err != nil {
			return nil, fmt.Errorf("parse %s/skill.yaml: %w", dirName, err)
		}
		if err := spec.validate(); err != nil {
			return nil, fmt.Errorf("validate %s/skill.yaml: %w", dirName, err)
		}

		entryData, err := fs.ReadFile(libraryFS, path.Join(base, spec.Entry))
		if err != nil {
			return nil, fmt.Errorf("load %s/%s: %w", dirName, spec.Entry, err)
		}

		references := make(map[string]string, len(spec.References))
		for _, ref := range spec.References {
			content, err := fs.ReadFile(libraryFS, path.Join(base, ref))
			if err != nil {
				return nil, fmt.Errorf("load %s/%s: %w", dirName, ref, err)
			}
			references[ref] = string(content)
		}

		metadata := ""
		if data, err := fs.ReadFile(libraryFS, path.Join(base, "agents/openai.yaml")); err == nil {
			metadata = string(data)
		}

		skill := &Skill{
			Spec:           spec,
			EntryMarkdown:  string(entryData),
			References:     references,
			OpenAIMetadata: metadata,
		}
		catalog[spec.ID] = skill
	}

	if len(catalog) == 0 {
		return nil, fmt.Errorf("embedded skill library is empty")
	}

	return catalog, nil
}

// SortedSummaries returns catalog summaries sorted by skill ID.
func SortedSummaries(catalog map[string]*Skill) []Summary {
	summaries := make([]Summary, 0, len(catalog))
	for _, skill := range catalog {
		summaries = append(summaries, Summary{
			Name:    skill.Spec.ID,
			Title:   skill.Spec.Title,
			Version: skill.Spec.Version,
			Summary: skill.Spec.Summary,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return strings.Compare(summaries[i].Name, summaries[j].Name) < 0
	})
	return summaries
}
