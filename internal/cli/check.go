package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ravenscroftj/raven/internal/check"
	"github.com/ravenscroftj/raven/internal/pages"
	"github.com/ravenscroftj/raven/internal/parser"
	"github.com/ravenscroftj/raven/internal/schema"
	"github.com/ravenscroftj/raven/internal/vault"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	checkStrict        bool
	checkCreateMissing bool
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate the vault",
	Long:  `Checks all files for errors and warnings (type mismatches, broken references, etc.)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		fmt.Printf("Checking vault: %s\n", vaultPath)

		// Load schema
		s, err := schema.Load(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to load schema: %w", err)
		}

		var errorCount, warningCount, fileCount int
		var allDocs []*parser.ParsedDocument
		var allObjectIDs []string

		// First pass: parse all documents and collect object IDs
		err = vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
			fileCount++

			if result.Error != nil {
				fmt.Printf("ERROR: %s - %v\n", result.RelativePath, result.Error)
				errorCount++
				return nil
			}

			allDocs = append(allDocs, result.Document)
			for _, obj := range result.Document.Objects {
				allObjectIDs = append(allObjectIDs, obj.ID)
			}

			return nil
		})

		if err != nil {
			return fmt.Errorf("error walking vault: %w", err)
		}

		// Second pass: validate with full context
		validator := check.NewValidator(s, allObjectIDs)

		for _, doc := range allDocs {
			issues := validator.ValidateDocument(doc)

			for _, issue := range issues {
				prefix := "ERROR"
				if issue.Level == check.LevelWarning {
					prefix = "WARN"
					warningCount++
				} else {
					errorCount++
				}

				fmt.Printf("%s:  %s:%d - %s\n", prefix, issue.FilePath, issue.Line, issue.Message)
			}
		}

		fmt.Println()
		if errorCount == 0 && warningCount == 0 {
			fmt.Printf("✓ No issues found in %d files.\n", fileCount)
		} else {
			fmt.Printf("Found %d error(s), %d warning(s) in %d files.\n", errorCount, warningCount, fileCount)
		}

		// Handle --create-missing
		if checkCreateMissing {
			missingRefs := validator.MissingRefs()
			if len(missingRefs) > 0 {
				created := handleMissingRefs(vaultPath, s, missingRefs)
				if created > 0 {
					fmt.Printf("\n✓ Created %d missing page(s).\n", created)
				}
			}

			undefinedTraits := validator.UndefinedTraits()
			if len(undefinedTraits) > 0 {
				added := handleUndefinedTraits(vaultPath, s, undefinedTraits)
				if added > 0 {
					fmt.Printf("\n✓ Added %d trait(s) to schema.\n", added)
				}
			}
		}

		if errorCount > 0 || (checkStrict && warningCount > 0) {
			os.Exit(1)
		}

		return nil
	},
}

func handleMissingRefs(vaultPath string, s *schema.Schema, refs []*check.MissingRef) int {
	// Categorize refs by confidence
	var certain, inferred, unknown []*check.MissingRef
	for _, ref := range refs {
		switch ref.Confidence {
		case check.ConfidenceCertain:
			certain = append(certain, ref)
		case check.ConfidenceInferred:
			inferred = append(inferred, ref)
		default:
			unknown = append(unknown, ref)
		}
	}

	// Sort each category by path for consistent output
	sortRefs := func(refs []*check.MissingRef) {
		sort.Slice(refs, func(i, j int) bool {
			return refs[i].TargetPath < refs[j].TargetPath
		})
	}
	sortRefs(certain)
	sortRefs(inferred)
	sortRefs(unknown)

	fmt.Println("\n--- Missing References ---")
	reader := bufio.NewReader(os.Stdin)
	created := 0

	// Handle certain refs (from typed fields)
	if len(certain) > 0 {
		fmt.Println("\nCertain (from typed fields):")
		for _, ref := range certain {
			source := ref.SourceObjectID
			if source == "" {
				source = ref.SourceFile
			}
			fmt.Printf("  • %s → %s (from %s.%s)\n", ref.TargetPath, ref.InferredType, source, ref.FieldSource)
		}

		fmt.Print("\nCreate these pages? [Y/n] ")
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response == "" || response == "y" || response == "yes" {
			for _, ref := range certain {
				sluggedPath := pages.SlugifyPath(ref.TargetPath)
				if err := createMissingPage(vaultPath, s, ref.TargetPath, ref.InferredType); err != nil {
					fmt.Printf("  ✗ Failed to create %s: %v\n", sluggedPath, err)
				} else {
					fmt.Printf("  ✓ Created %s.md (type: %s)\n", sluggedPath, ref.InferredType)
					created++
				}
			}
		}
	}

	// Handle inferred refs (from path matching)
	if len(inferred) > 0 {
		fmt.Println("\nInferred (from path matching default_path):")
		for _, ref := range inferred {
			fmt.Printf("  ? %s → %s (inferred from path)\n", ref.TargetPath, ref.InferredType)
		}

		for _, ref := range inferred {
			sluggedPath := pages.SlugifyPath(ref.TargetPath)
			fmt.Printf("\nCreate %s as '%s'? [y/N] ", sluggedPath, ref.InferredType)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))
			if response == "y" || response == "yes" {
				if err := createMissingPage(vaultPath, s, ref.TargetPath, ref.InferredType); err != nil {
					fmt.Printf("  ✗ Failed to create %s: %v\n", sluggedPath, err)
				} else {
					fmt.Printf("  ✓ Created %s.md (type: %s)\n", sluggedPath, ref.InferredType)
					created++
				}
			}
		}
	}

	// Handle unknown refs
	if len(unknown) > 0 {
		fmt.Println("\nUnknown type (please specify):")
		for _, ref := range unknown {
			fmt.Printf("  ? %s (referenced in %s:%d)\n", ref.TargetPath, ref.SourceFile, ref.Line)
		}

		// List available types
		var typeNames []string
		for name := range s.Types {
			typeNames = append(typeNames, name)
		}
		sort.Strings(typeNames)
		fmt.Printf("\nAvailable types: %s\n", strings.Join(typeNames, ", "))

		for _, ref := range unknown {
			sluggedPath := pages.SlugifyPath(ref.TargetPath)
			fmt.Printf("\nType for %s (or 'skip'): ", ref.TargetPath)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(response)

			if response == "" || response == "skip" || response == "s" {
				fmt.Printf("  Skipped %s\n", ref.TargetPath)
				continue
			}

			// Validate type exists, offer to create if not
			if _, exists := s.Types[response]; !exists {
				created += handleNewTypeCreation(vaultPath, s, ref, response, sluggedPath, reader)
				continue
			}

			if err := createMissingPage(vaultPath, s, ref.TargetPath, response); err != nil {
				fmt.Printf("  ✗ Failed to create %s: %v\n", sluggedPath, err)
			} else {
				fmt.Printf("  ✓ Created %s.md (type: %s)\n", sluggedPath, response)
				created++
			}
		}
	}

	return created
}

// handleUndefinedTraits prompts the user to add undefined traits to the schema.
// Returns the number of traits added.
func handleUndefinedTraits(vaultPath string, s *schema.Schema, traits []*check.UndefinedTrait) int {
	if len(traits) == 0 {
		return 0
	}

	// Sort by usage count (most used first)
	sort.Slice(traits, func(i, j int) bool {
		return traits[i].UsageCount > traits[j].UsageCount
	})

	fmt.Println("\n--- Undefined Traits ---")
	fmt.Println("\nThe following traits are used but not defined in schema.yaml:")
	for _, trait := range traits {
		valueInfo := "no value"
		if trait.HasValue {
			valueInfo = "with value"
		}
		fmt.Printf("  • @%s (%d usages, %s)\n", trait.TraitName, trait.UsageCount, valueInfo)
		for _, loc := range trait.Locations {
			fmt.Printf("      %s\n", loc)
		}
	}

	reader := bufio.NewReader(os.Stdin)
	added := 0

	fmt.Println("\nWould you like to add these traits to the schema?")

	for _, trait := range traits {
		fmt.Printf("\nAdd '@%s' to schema? [y/N] ", trait.TraitName)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			fmt.Printf("  Skipped @%s\n", trait.TraitName)
			continue
		}

		// Determine trait type
		traitType := promptTraitType(trait, reader)
		if traitType == "" {
			fmt.Printf("  Skipped @%s\n", trait.TraitName)
			continue
		}

		// Get additional options based on type
		var enumValues []string
		var defaultValue string

		if traitType == "enum" {
			fmt.Printf("  Enum values (comma-separated, e.g., 'low,medium,high'): ")
			valuesStr, _ := reader.ReadString('\n')
			valuesStr = strings.TrimSpace(valuesStr)
			if valuesStr != "" {
				enumValues = strings.Split(valuesStr, ",")
				for i := range enumValues {
					enumValues[i] = strings.TrimSpace(enumValues[i])
				}
			}
		}

		if traitType == "boolean" || traitType == "enum" {
			fmt.Printf("  Default value (or leave empty): ")
			defaultValue, _ = reader.ReadString('\n')
			defaultValue = strings.TrimSpace(defaultValue)
		}

		// Create the trait
		if err := createNewTrait(vaultPath, s, trait.TraitName, traitType, enumValues, defaultValue); err != nil {
			fmt.Printf("  ✗ Failed to add @%s: %v\n", trait.TraitName, err)
			continue
		}

		fmt.Printf("  ✓ Added trait '@%s' (type: %s) to schema.yaml\n", trait.TraitName, traitType)
		added++
	}

	return added
}

// promptTraitType asks the user what type a trait should be.
func promptTraitType(trait *check.UndefinedTrait, reader *bufio.Reader) string {
	// Suggest a type based on usage
	suggested := "boolean"
	if trait.HasValue {
		suggested = "string"
	}

	fmt.Printf("  Type for @%s? [boolean/string/date/enum] (default: %s): ", trait.TraitName, suggested)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response == "" {
		return suggested
	}

	validTypes := map[string]bool{
		"boolean": true, "bool": true,
		"string": true,
		"date": true,
		"datetime": true,
		"enum": true,
		"ref": true,
	}

	// Normalize bool -> boolean
	if response == "bool" {
		response = "boolean"
	}

	if !validTypes[response] {
		fmt.Printf("  Invalid type '%s'\n", response)
		return ""
	}

	return response
}

// createNewTrait adds a new trait to schema.yaml.
func createNewTrait(vaultPath string, s *schema.Schema, traitName, traitType string, enumValues []string, defaultValue string) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Read current schema file
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to read schema: %w", err)
	}

	// Parse as YAML to modify
	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}

	// Ensure traits map exists
	traits, ok := schemaDoc["traits"].(map[string]interface{})
	if !ok {
		traits = make(map[string]interface{})
		schemaDoc["traits"] = traits
	}

	// Build new trait definition
	newTrait := make(map[string]interface{})
	newTrait["type"] = traitType

	if len(enumValues) > 0 {
		newTrait["values"] = enumValues
	}

	if defaultValue != "" {
		// Convert "true"/"false" to boolean for boolean traits
		if traitType == "boolean" {
			if defaultValue == "true" {
				newTrait["default"] = true
			} else if defaultValue == "false" {
				newTrait["default"] = false
			}
		} else {
			newTrait["default"] = defaultValue
		}
	}

	traits[traitName] = newTrait

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}

	if err := os.WriteFile(schemaPath, output, 0644); err != nil {
		return fmt.Errorf("failed to write schema: %w", err)
	}

	// Update the in-memory schema
	s.Traits[traitName] = &schema.TraitDefinition{
		Type:   schema.FieldType(traitType),
		Values: enumValues,
	}
	if defaultValue != "" {
		if traitType == "boolean" {
			s.Traits[traitName].Default = defaultValue == "true"
		} else {
			s.Traits[traitName].Default = defaultValue
		}
	}

	return nil
}

// handleNewTypeCreation prompts the user to create a new type when they enter a type that doesn't exist.
// Returns the number of pages created (0 or 1).
func handleNewTypeCreation(vaultPath string, s *schema.Schema, ref *check.MissingRef, typeName, sluggedPath string, reader *bufio.Reader) int {
	fmt.Printf("\n  Type '%s' doesn't exist. Would you like to create it? [y/N] ", typeName)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "y" && response != "yes" {
		fmt.Printf("  Skipped %s\n", ref.TargetPath)
		return 0
	}

	// Prompt for default_path (optional)
	fmt.Printf("  Default path for '%s' files (e.g., '%s/', or leave empty): ", typeName, typeName+"s")
	defaultPath, _ := reader.ReadString('\n')
	defaultPath = strings.TrimSpace(defaultPath)

	// Create the type
	if err := createNewType(vaultPath, s, typeName, defaultPath); err != nil {
		fmt.Printf("  ✗ Failed to create type '%s': %v\n", typeName, err)
		return 0
	}
	fmt.Printf("  ✓ Created type '%s' in schema.yaml\n", typeName)
	if defaultPath != "" {
		fmt.Printf("    default_path: %s\n", defaultPath)
	}

	// Now create the page with the new type
	if err := createMissingPage(vaultPath, s, ref.TargetPath, typeName); err != nil {
		fmt.Printf("  ✗ Failed to create %s: %v\n", sluggedPath, err)
		return 0
	}
	fmt.Printf("  ✓ Created %s.md (type: %s)\n", sluggedPath, typeName)
	return 1
}

// createNewType adds a new type to schema.yaml.
func createNewType(vaultPath string, s *schema.Schema, typeName, defaultPath string) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Check built-in types
	if typeName == "page" || typeName == "section" || typeName == "date" {
		return fmt.Errorf("'%s' is a built-in type", typeName)
	}

	// Read current schema file
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to read schema: %w", err)
	}

	// Parse as YAML to modify
	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}

	// Ensure types map exists
	types, ok := schemaDoc["types"].(map[string]interface{})
	if !ok {
		types = make(map[string]interface{})
		schemaDoc["types"] = types
	}

	// Build new type definition
	newType := make(map[string]interface{})
	if defaultPath != "" {
		newType["default_path"] = defaultPath
	}

	types[typeName] = newType

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}

	if err := os.WriteFile(schemaPath, output, 0644); err != nil {
		return fmt.Errorf("failed to write schema: %w", err)
	}

	// Update the in-memory schema so subsequent page creation works
	s.Types[typeName] = &schema.TypeDefinition{
		DefaultPath: defaultPath,
	}

	return nil
}

// createMissingPage creates a new page file using the pages package.
func createMissingPage(vaultPath string, s *schema.Schema, targetPath, typeName string) error {
	_, err := pages.Create(pages.CreateOptions{
		VaultPath:                   vaultPath,
		TypeName:                    typeName,
		TargetPath:                  targetPath,
		Schema:                      s,
		IncludeRequiredPlaceholders: true,
	})
	return err
}

func init() {
	checkCmd.Flags().BoolVar(&checkStrict, "strict", false, "Treat warnings as errors")
	checkCmd.Flags().BoolVar(&checkCreateMissing, "create-missing", false, "Interactively create missing referenced pages")
	rootCmd.AddCommand(checkCmd)
}
