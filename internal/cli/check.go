package cli

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ravenscroftj/raven/internal/check"
	"github.com/ravenscroftj/raven/internal/pages"
	"github.com/ravenscroftj/raven/internal/parser"
	"github.com/ravenscroftj/raven/internal/schema"
	"github.com/ravenscroftj/raven/internal/vault"
	"github.com/spf13/cobra"
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

			// Validate type exists
			if _, exists := s.Types[response]; !exists {
				fmt.Printf("  ✗ Unknown type '%s', skipping %s\n", response, ref.TargetPath)
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
