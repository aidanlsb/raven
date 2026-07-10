package skills

import (
	"sort"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/commands"
)

func TestSkillCoverageCommandsExistInRegistry(t *testing.T) {
	t.Parallel()
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}

	registryNames := make(map[string]struct{}, len(commands.Registry))
	for _, meta := range commands.Registry {
		name := strings.TrimSpace(meta.Name)
		if name == "" {
			continue
		}
		registryNames[name] = struct{}{}
	}

	for _, skillID := range sortedSkillIDs(catalog) {
		skill := catalog[skillID]
		if len(skill.Spec.Coverage.Commands) == 0 {
			t.Errorf("%s has no coverage.commands entries", skillID)
			continue
		}
		for _, commandName := range skill.Spec.Coverage.Commands {
			if _, ok := registryNames[strings.TrimSpace(commandName)]; !ok {
				t.Errorf("%s coverage.commands contains unknown command %q", skillID, commandName)
			}
		}
	}
}

func TestRenderFilesIncludesDeclaredReferences(t *testing.T) {
	t.Parallel()
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}

	for _, skillID := range sortedSkillIDs(catalog) {
		skill := catalog[skillID]
		rendered, err := RenderFiles(skill)
		if err != nil {
			t.Fatalf("RenderFiles(%s) error = %v", skillID, err)
		}

		if _, ok := rendered["SKILL.md"]; !ok {
			t.Errorf("RenderFiles(%s) missing SKILL.md", skillID)
		}

		for _, ref := range skill.Spec.References {
			if _, ok := rendered[ref]; !ok {
				t.Errorf("RenderFiles(%s) missing declared reference %q", skillID, ref)
			}
		}

		_, hasOpenAIMetadata := rendered["agents/openai.yaml"]
		wantOpenAIMetadata := strings.TrimSpace(skill.OpenAIMetadata) != ""
		if hasOpenAIMetadata != wantOpenAIMetadata {
			t.Errorf("RenderFiles(%s) agents/openai.yaml present = %v, want %v", skillID, hasOpenAIMetadata, wantOpenAIMetadata)
		}
	}
}

func TestSkillMarkdownDoesNotUseLegacySyntax(t *testing.T) {
	t.Parallel()
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}

	legacyTokens := []string{
		"has:{",
		"contains:{",
		"within:{",
		"on:{",
		"refs:{",
		"refs:[[",
		`content:"`,
	}

	for _, skillID := range sortedSkillIDs(catalog) {
		skill := catalog[skillID]
		files := map[string]string{
			skill.Spec.Entry: skill.EntryMarkdown,
		}
		for refPath, content := range skill.References {
			files[refPath] = content
		}

		for _, filePath := range sortedStringKeys(files) {
			content := files[filePath]
			for _, token := range legacyTokens {
				if strings.Contains(content, token) {
					t.Errorf("%s file %s contains legacy token %q", skillID, filePath, token)
				}
			}
		}
	}
}

func sortedSkillIDs(catalog map[string]*Skill) []string {
	ids := make([]string, 0, len(catalog))
	for id := range catalog {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
