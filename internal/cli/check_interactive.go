package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/checksvc"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
)

// renderCanonicalCheckCreateMissing runs the interactive create-missing flow in
// non-JSON mode. Prompts stay here in the CLI; all schema/file mutations are
// delegated to checksvc appliers via collectMissingRefDecisions +
// checksvc.ApplyMissingRefResolutions (and the trait equivalents).
func renderCanonicalCheckCreateMissing(vaultPath string, result commandexec.Result) error {
	data := canonicalDataMap(result)
	missingRefs := decodeMissingRefs(data["missing_ref_items"])
	undefinedTraits := decodeUndefinedTraits(data["undefined_trait_items"])

	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
	}
	s, err := loadSchemaSafe(vaultPath)
	if err != nil {
		return fmt.Errorf("failed to load schema: %w", err)
	}

	if len(missingRefs) > 0 {
		interaction := newCheckInteraction(os.Stdin, os.Stdout)
		created := runMissingRefsInteractive(vaultPath, s, missingRefs, interaction, vaultCfg)
		if created > 0 {
			fmt.Printf("\n%s\n", ui.Checkf("Created %d missing page(s).", created))
		}
		added := 0
		if len(undefinedTraits) > 0 {
			added = runUndefinedTraitsInteractive(vaultPath, s, undefinedTraits, interaction)
		}
		if added > 0 {
			fmt.Printf("\n%s\n", ui.Checkf("Added %d trait(s) to schema.", added))
		}
		return nil
	}
	if len(undefinedTraits) > 0 {
		interaction := newCheckInteraction(os.Stdin, os.Stdout)
		added := runUndefinedTraitsInteractive(vaultPath, s, undefinedTraits, interaction)
		if added > 0 {
			fmt.Printf("\n%s\n", ui.Checkf("Added %d trait(s) to schema.", added))
		}
	}
	return nil
}

// promptCreateMissingRefsFromResult inspects a successful write result for
// missing reference targets and, in interactive (non-JSON) mode, offers to
// create the missing pages. Writes remain permissive: the object was already
// created/modified; this is purely additive UX layered on top of the completed
// write, reusing the same interactive flow as `rvn check create-missing`.
func promptCreateMissingRefsFromResult(vaultPath string, result commandexec.Result) {
	if jsonOutput || !canUseInteractiveTerminal() {
		return
	}
	data := canonicalDataMap(result)
	missingRefs := decodeMissingRefs(data["missing_ref_items"])
	if len(missingRefs) == 0 {
		return
	}

	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return
	}
	s, err := loadSchemaSafe(vaultPath)
	if err != nil {
		return
	}

	interaction := newCheckInteraction(os.Stdin, os.Stdout)
	created := runMissingRefsInteractive(vaultPath, s, missingRefs, interaction, vaultCfg)
	if created > 0 {
		fmt.Printf("\n%s\n", ui.Checkf("Created %d missing page(s).", created))
	}
}

func decodeMissingRefs(raw interface{}) []*check.MissingRef {
	var refs []*check.MissingRef
	decodeCanonicalValue(raw, &refs)
	return refs
}

func decodeUndefinedTraits(raw interface{}) []*check.UndefinedTrait {
	var traits []*check.UndefinedTrait
	decodeCanonicalValue(raw, &traits)
	return traits
}

func decodeCanonicalValue(raw interface{}, target interface{}) bool {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return false
	}
	return json.Unmarshal(encoded, target) == nil
}

// runMissingRefsInteractive prompts for missing-reference creation decisions and
// delegates the resulting schema/file mutations to checksvc. It returns the
// number of pages created.
func runMissingRefsInteractive(vaultPath string, s *schema.Schema, refs []*check.MissingRef, interaction checkInteraction, vaultCfg *config.VaultConfig) int {
	newTypes, resolutions := collectMissingRefDecisions(s, refs, interaction, vaultCfg)
	if len(newTypes) == 0 && len(resolutions) == 0 {
		return 0
	}
	applied := checksvc.ApplyMissingRefResolutions(vaultPath, s, newTypes, resolutions, vaultCfg)
	return renderMissingRefOutcomes(interaction, applied)
}

// collectMissingRefDecisions runs the interactive missing-reference prompts and
// returns the user's resolved decisions. It performs no mutations: new types
// the user asks to create are returned as NewTypeResolutions, and pages to
// create are returned as MissingRefResolutions.
func collectMissingRefDecisions(s *schema.Schema, refs []*check.MissingRef, interaction checkInteraction, vaultCfg *config.VaultConfig) ([]checksvc.NewTypeResolution, []checksvc.MissingRefResolution) {
	groups := checksvc.GroupMissingRefsForInteractive(refs)

	interaction.Printf("\n%s\n", ui.SectionHeader("Missing References"))

	objectsRoot := vaultCfg.GetObjectsRoot()
	pagesRoot := vaultCfg.GetPagesRoot()
	dailyDir := vaultCfg.GetDailyDirectory()
	resolvePath := func(targetPath, typeName string) string {
		return checksvc.ResolveAndSlugifyTargetPath(targetPath, typeName, s, objectsRoot, pagesRoot, dailyDir)
	}

	var newTypes []checksvc.NewTypeResolution
	var resolutions []checksvc.MissingRefResolution
	// pendingTypes lets a second unknown ref reuse a type the user already
	// chose to create in this session (mirrors the old in-place schema update).
	pendingTypes := map[string]struct{}{}

	// Handle certain refs (from typed fields).
	if len(groups.Certain) > 0 {
		interaction.Printf("\n%s\n", ui.Bold.Render("Certain (from typed fields):"))
		for _, ref := range groups.Certain {
			source := ref.SourceObjectID
			if source == "" {
				source = ref.SourceFile
			}
			resolvedPath := resolvePath(ref.TargetPath, ref.InferredType)
			item := fmt.Sprintf("%s → %s %s",
				ui.Bold.Render(ref.TargetPath),
				ui.FilePath(resolvedPath+".md"),
				ui.Muted.Render(fmt.Sprintf("(from %s.%s)", source, ref.FieldSource)))
			interaction.Println(ui.Bullet(item))
		}

		interaction.Printf("\nCreate these pages? %s ", ui.Muted.Render("[Y/n]"))
		response := readTrimmedLowerLine(interaction)
		if response == "" || response == "y" || response == "yes" {
			for _, ref := range groups.Certain {
				resolutions = append(resolutions, checksvc.MissingRefResolution{TargetPath: ref.TargetPath, TypeName: ref.InferredType})
			}
		}
	}

	// Handle inferred refs (from path matching).
	if len(groups.Inferred) > 0 {
		interaction.Printf("\n%s\n", ui.Bold.Render("Inferred (from path matching default_path):"))
		for _, ref := range groups.Inferred {
			resolvedPath := resolvePath(ref.TargetPath, ref.InferredType)
			item := fmt.Sprintf("? %s → %s %s",
				ui.Bold.Render(ref.TargetPath),
				ui.FilePath(resolvedPath+".md"),
				ui.Muted.Render(fmt.Sprintf("(type: %s)", ref.InferredType)))
			interaction.Println(ui.Bullet(item))
		}

		for _, ref := range groups.Inferred {
			resolvedPath := resolvePath(ref.TargetPath, ref.InferredType)
			interaction.Printf("\nCreate %s as '%s'? %s ", ui.FilePath(resolvedPath+".md"), ui.Bold.Render(ref.InferredType), ui.Muted.Render("[y/N]"))
			response := readTrimmedLowerLine(interaction)
			if response == "y" || response == "yes" {
				resolutions = append(resolutions, checksvc.MissingRefResolution{TargetPath: ref.TargetPath, TypeName: ref.InferredType})
			}
		}
	}

	// Handle unknown refs.
	if len(groups.Unknown) > 0 {
		interaction.Printf("\n%s\n", ui.Bold.Render("Unknown type (please specify):"))
		for _, ref := range groups.Unknown {
			item := fmt.Sprintf("? %s %s",
				ui.Bold.Render(ref.TargetPath),
				ui.Muted.Render(fmt.Sprintf("(referenced in %s:%d)", ref.SourceFile, ref.Line)))
			interaction.Println(ui.Bullet(item))
		}

		typeNames := checksvc.AvailableTypeNames(s)
		interaction.Printf("\nAvailable types: %s\n", ui.Bold.Render(strings.Join(typeNames, ", ")))

		for _, ref := range groups.Unknown {
			interaction.Printf("\nType for %s %s: ", ui.Bold.Render(ref.TargetPath), ui.Muted.Render("(or 'skip')"))
			response := readTrimmedLine(interaction)

			if response == "" || response == "skip" || response == "s" {
				interaction.Printf("  %s\n", ui.Muted.Render("Skipped "+ref.TargetPath))
				continue
			}

			// Offer to create the type when it is neither defined, built-in,
			// nor already queued for creation in this session.
			_, definedInSchema := s.Types[response]
			_, queued := pendingTypes[response]
			if !definedInSchema && !queued && !schema.IsBuiltinType(response) {
				create, defaultPath := promptNewTypeCreation(response, ref, interaction)
				if !create {
					continue
				}
				newTypes = append(newTypes, checksvc.NewTypeResolution{TypeName: response, DefaultPath: defaultPath})
				pendingTypes[response] = struct{}{}
			}

			resolutions = append(resolutions, checksvc.MissingRefResolution{TargetPath: ref.TargetPath, TypeName: response})
		}
	}

	return newTypes, resolutions
}

// renderMissingRefOutcomes prints the results of applying missing-ref decisions
// and returns the number of pages created.
func renderMissingRefOutcomes(interaction checkInteraction, applied checksvc.MissingRefApplyResult) int {
	for _, typeOutcome := range applied.Types {
		if typeOutcome.Err != nil {
			interaction.Printf("  %s\n", ui.Errorf("Failed to create type '%s': %v", typeOutcome.TypeName, typeOutcome.Err))
			continue
		}
		interaction.Printf("  %s\n", ui.Checkf("Created type '%s' in schema.yaml", typeOutcome.TypeName))
		if typeOutcome.DefaultPath != "" {
			interaction.Printf("    %s\n", ui.Muted.Render("default_path: "+typeOutcome.DefaultPath))
		}
	}

	created := 0
	for _, page := range applied.Pages {
		if page.Err != nil {
			interaction.Printf("  %s\n", ui.Errorf("Failed to create %s.md: %v", page.ResolvedPath, page.Err))
			continue
		}
		interaction.Printf("  %s\n", ui.Checkf("Created %s.md (type: %s)", page.ResolvedPath, page.TypeName))
		created++
	}
	return created
}

// runUndefinedTraitsInteractive prompts for undefined-trait decisions and
// delegates the schema mutations to checksvc. It returns the number of traits
// added.
func runUndefinedTraitsInteractive(vaultPath string, s *schema.Schema, traits []*check.UndefinedTrait, interaction checkInteraction) int {
	resolutions := collectTraitDecisions(traits, interaction)
	if len(resolutions) == 0 {
		return 0
	}
	outcomes := checksvc.ApplyTraitResolutions(vaultPath, s, resolutions)
	return renderTraitOutcomes(interaction, outcomes)
}

// collectTraitDecisions prompts the user about undefined traits and returns the
// resolved decisions. It performs no mutations.
func collectTraitDecisions(traits []*check.UndefinedTrait, interaction checkInteraction) []checksvc.TraitResolution {
	if len(traits) == 0 {
		return nil
	}

	sort.Slice(traits, func(i, j int) bool {
		return traits[i].UsageCount > traits[j].UsageCount
	})

	interaction.Printf("\n%s\n", ui.SectionHeader("Undefined Traits"))
	interaction.Println("\nThe following traits are used but not defined in schema.yaml:")
	for _, trait := range traits {
		valueInfo := "no value"
		if trait.HasValue {
			valueInfo = "with value"
		}
		item := fmt.Sprintf("%s %s",
			ui.Bold.Render("@"+trait.TraitName),
			ui.Muted.Render(fmt.Sprintf("(%d usages, %s)", trait.UsageCount, valueInfo)))
		interaction.Println(ui.Bullet(item))
		for _, loc := range trait.Locations {
			interaction.Printf("      %s\n", ui.Muted.Render(loc))
		}
	}

	interaction.Println("\nWould you like to add these traits to the schema?")

	var resolutions []checksvc.TraitResolution
	for _, trait := range traits {
		interaction.Printf("\nAdd %s to schema? %s ", ui.Bold.Render("@"+trait.TraitName), ui.Muted.Render("[y/N]"))
		response := readTrimmedLowerLine(interaction)
		if response != "y" && response != "yes" {
			interaction.Printf("  %s\n", ui.Muted.Render("Skipped @"+trait.TraitName))
			continue
		}

		traitType := promptTraitType(trait, interaction)
		if traitType == "" {
			interaction.Printf("  %s\n", ui.Muted.Render("Skipped @"+trait.TraitName))
			continue
		}

		var enumValues []string
		var defaultValue string
		if traitType == "enum" {
			interaction.Printf("  Enum values %s: ", ui.Muted.Render("(comma-separated, e.g., 'low,medium,high')"))
			valuesStr := readTrimmedLine(interaction)
			if valuesStr != "" {
				enumValues = strings.Split(valuesStr, ",")
				for i := range enumValues {
					enumValues[i] = strings.TrimSpace(enumValues[i])
				}
			}
		}

		if traitType == "boolean" || traitType == "enum" {
			interaction.Printf("  Default value %s: ", ui.Muted.Render("(or leave empty)"))
			defaultValue = readTrimmedLine(interaction)
		}

		resolutions = append(resolutions, checksvc.TraitResolution{
			TraitName:    trait.TraitName,
			TraitType:    traitType,
			EnumValues:   enumValues,
			DefaultValue: defaultValue,
		})
	}

	return resolutions
}

// renderTraitOutcomes prints the results of applying trait decisions and returns
// the number of traits added.
func renderTraitOutcomes(interaction checkInteraction, outcomes []checksvc.TraitOutcome) int {
	added := 0
	for _, outcome := range outcomes {
		if outcome.Err != nil {
			interaction.Printf("  %s\n", ui.Errorf("Failed to add @%s: %v", outcome.TraitName, outcome.Err))
			continue
		}
		interaction.Printf("  %s\n", ui.Checkf("Added trait '@%s' (type: %s) to schema.yaml", outcome.TraitName, outcome.TraitType))
		added++
	}
	return added
}

// promptTraitType asks the user what type a trait should be.
func promptTraitType(trait *check.UndefinedTrait, interaction checkInteraction) string {
	suggested := "boolean"
	if trait.HasValue {
		suggested = "string"
	}

	interaction.Printf("  Type for %s? %s %s: ",
		ui.Bold.Render("@"+trait.TraitName),
		ui.Muted.Render("[boolean/string/number/date/datetime/enum/ref/url]"),
		ui.Muted.Render(fmt.Sprintf("(default: %s)", suggested)))
	response := readTrimmedLowerLine(interaction)
	if response == "" {
		return suggested
	}

	validTypes := map[string]bool{
		"boolean": true, "bool": true,
		"string":   true,
		"number":   true,
		"date":     true,
		"datetime": true,
		"enum":     true,
		"ref":      true,
		"url":      true,
	}
	if response == "bool" {
		response = "boolean"
	}
	if !validTypes[response] {
		interaction.Printf("  %s\n", ui.Errorf("Invalid type '%s'", response))
		return ""
	}
	return response
}

// promptNewTypeCreation asks the user whether to create a type that does not yet
// exist and, if so, for its default path. It performs no mutations; the caller
// records the decision and checksvc applies it. Returns create=false when the
// user declines (the referencing page is then skipped).
func promptNewTypeCreation(typeName string, ref *check.MissingRef, interaction checkInteraction) (create bool, defaultPath string) {
	interaction.Printf("\n  Type %s doesn't exist. Would you like to create it? %s ",
		ui.Bold.Render("'"+typeName+"'"),
		ui.Muted.Render("[y/N]"))
	response := readTrimmedLowerLine(interaction)

	if response != "y" && response != "yes" {
		interaction.Printf("  %s\n", ui.Muted.Render("Skipped "+ref.TargetPath))
		return false, ""
	}

	interaction.Printf("  Default path for '%s' files %s: ", typeName, ui.Muted.Render(fmt.Sprintf("(e.g., '%s/', or leave empty)", typeName+"s")))
	defaultPath = readTrimmedLine(interaction)
	return true, defaultPath
}
