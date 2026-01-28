package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/workflow"
)

var workflowApplyPlanCmd = &cobra.Command{
	Use:   "apply-plan <name>",
	Short: "Apply a workflow plan (preview by default)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		name := args[0]

		vaultCfg := loadVaultConfigSafe(vaultPath)

		// Ensure the workflow exists (future: per-workflow guardrails).
		if _, err := workflow.Get(vaultPath, name, vaultCfg); err != nil {
			return handleError(ErrQueryNotFound, err, "Use 'rvn workflow list' to see available workflows")
		}

		if workflowPlanFile == "" {
			return handleErrorMsg(ErrMissingArgument, "missing --plan", "Usage: rvn workflow apply-plan <name> --plan <file.json> [--confirm]")
		}

		data, err := readPlanBytes(cmd, workflowPlanFile)
		if err != nil {
			return handleError(ErrInvalidInput, err, "")
		}

		plan, err := extractPlan(data)
		if err != nil {
			return handleError(ErrInvalidInput, err, "")
		}
		if err := workflow.ValidatePlan(plan); err != nil {
			return handleError(ErrInvalidInput, err, "")
		}

		preview, err := previewPlan(vaultPath, vaultCfg, plan)
		if err != nil {
			return handleError(ErrInvalidInput, err, "")
		}

		if !workflowApplyConfirm {
			if isJSONOutput() {
				outputSuccess(map[string]interface{}{
					"workflow": name,
					"confirm":  false,
					"preview":  preview,
				}, nil)
				return nil
			}
			printPlanPreview(name, preview)
			return nil
		}

		if err := applyPlan(vaultPath, vaultCfg, plan); err != nil {
			return err
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"workflow": name,
				"confirm":  true,
				"ok":       true,
			}, nil)
			return nil
		}

		fmt.Println("Applied plan.")
		return nil
	},
}

func init() {
	workflowApplyPlanCmd.Flags().StringVar(&workflowPlanFile, "plan", "", "Path to JSON prompt output (must include outputs.plan) or a plan object; use '-' for stdin")
	workflowApplyPlanCmd.Flags().BoolVar(&workflowApplyConfirm, "confirm", false, "Apply changes (without this flag, shows preview only)")
}

func readPlanBytes(cmd *cobra.Command, path string) ([]byte, error) {
	if path == "-" {
		b, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, err
		}
		return bytes.TrimSpace(b), nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return bytes.TrimSpace(b), nil
}

func extractPlan(data []byte) (*workflow.Plan, error) {
	// 1) Try prompt envelope with outputs.plan
	var env workflow.PromptEnvelope
	if err := json.Unmarshal(data, &env); err == nil && env.Outputs != nil {
		if raw, ok := env.Outputs["plan"]; ok {
			var p workflow.Plan
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, fmt.Errorf("invalid outputs.plan: %w", err)
			}
			return &p, nil
		}
		return nil, fmt.Errorf("prompt output missing outputs.plan")
	}

	// 2) Try parsing as a plan directly
	var p workflow.Plan
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("invalid JSON (expected prompt envelope or plan): %w", err)
	}
	return &p, nil
}

type planPreviewItem struct {
	Op      string                 `json:"op"`
	Why     string                 `json:"why"`
	Summary string                 `json:"summary"`
	Details map[string]interface{} `json:"details,omitempty"`
}

type planPreview struct {
	Items []planPreviewItem `json:"items"`
}

func previewPlan(vaultPath string, vaultCfg *config.VaultConfig, plan *workflow.Plan) (*planPreview, error) {
	out := &planPreview{}
	for i, op := range plan.Ops {
		item, err := previewOp(vaultPath, vaultCfg, op)
		if err != nil {
			return nil, fmt.Errorf("op[%d] (%s): %w", i, op.Op, err)
		}
		out.Items = append(out.Items, *item)
	}
	return out, nil
}

func previewOp(vaultPath string, vaultCfg *config.VaultConfig, op workflow.Op) (*planPreviewItem, error) {
	switch op.Op {
	case "add":
		var args workflow.AddArgs
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, err
		}
		if args.To == "" || args.Text == "" {
			return nil, fmt.Errorf("add requires to and text")
		}
		res, err := ResolveReference(args.To, ResolveOptions{VaultPath: vaultPath, VaultConfig: vaultCfg, AllowMissing: true})
		if err != nil {
			return nil, err
		}
		if isProtectedAbs(vaultPath, res.FilePath, vaultCfg) {
			return nil, fmt.Errorf("target is protected: %s", res.ObjectID)
		}
		summary := fmt.Sprintf("append to %s", res.ObjectID)
		if args.Heading != "" {
			summary += fmt.Sprintf(" under heading %q", args.Heading)
		}
		return &planPreviewItem{Op: op.Op, Why: op.Why, Summary: summary}, nil

	case "edit":
		var args workflow.EditArgs
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, err
		}
		if args.Path == "" || args.OldStr == "" {
			return nil, fmt.Errorf("edit requires path and old_str")
		}
		abs := filepath.Join(vaultPath, args.Path)
		if err := paths.ValidateWithinVault(vaultPath, abs); err != nil {
			return nil, err
		}
		if isProtectedAbs(vaultPath, abs, vaultCfg) {
			return nil, fmt.Errorf("path is protected: %s", args.Path)
		}
		b, err := os.ReadFile(abs)
		if err != nil {
			return nil, err
		}
		count := strings.Count(string(b), args.OldStr)
		if count != 1 {
			return nil, fmt.Errorf("old_str must match exactly once (matches=%d)", count)
		}
		return &planPreviewItem{Op: op.Op, Why: op.Why, Summary: fmt.Sprintf("edit %s (1 replacement)", args.Path)}, nil

	case "set":
		var args workflow.SetArgs
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, err
		}
		if args.ObjectID == "" || len(args.Fields) == 0 {
			return nil, fmt.Errorf("set requires object_id and fields")
		}
		res, err := ResolveReference(args.ObjectID, ResolveOptions{VaultPath: vaultPath, VaultConfig: vaultCfg})
		if err != nil {
			return nil, err
		}
		if isProtectedAbs(vaultPath, res.FilePath, vaultCfg) {
			return nil, fmt.Errorf("target is protected: %s", args.ObjectID)
		}
		return &planPreviewItem{
			Op:      op.Op,
			Why:     op.Why,
			Summary: fmt.Sprintf("set %d field(s) on %s", len(args.Fields), res.ObjectID),
		}, nil

	case "move":
		var args workflow.MoveArgs
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, err
		}
		if args.Source == "" || args.Destination == "" {
			return nil, fmt.Errorf("move requires source and destination")
		}
		src, err := ResolveReference(args.Source, ResolveOptions{VaultPath: vaultPath, VaultConfig: vaultCfg})
		if err != nil {
			return nil, err
		}
		if isProtectedAbs(vaultPath, src.FilePath, vaultCfg) {
			return nil, fmt.Errorf("source is protected: %s", args.Source)
		}
		relDest := resolveDestinationRelPath(vaultCfg, args.Destination)
		if paths.IsProtectedRelPath(relDest, vaultCfg.ProtectedPrefixes) {
			return nil, fmt.Errorf("destination is protected: %s", relDest)
		}
		return &planPreviewItem{
			Op:      op.Op,
			Why:     op.Why,
			Summary: fmt.Sprintf("move %s -> %s", src.ObjectID, args.Destination),
		}, nil

	case "update_trait":
		var args workflow.UpdateTraitArgs
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, err
		}
		if args.TraitID == "" || args.Value == "" {
			return nil, fmt.Errorf("update_trait requires trait_id and value")
		}
		fileRel, _, err := parseTraitID(args.TraitID)
		if err != nil {
			return nil, err
		}
		if paths.IsProtectedRelPath(fileRel, vaultCfg.ProtectedPrefixes) {
			return nil, fmt.Errorf("trait file is protected: %s", fileRel)
		}
		return &planPreviewItem{
			Op:      op.Op,
			Why:     op.Why,
			Summary: fmt.Sprintf("update %s -> %s", args.TraitID, args.Value),
		}, nil
	default:
		return nil, fmt.Errorf("unknown op: %s", op.Op)
	}
}

func applyPlan(vaultPath string, vaultCfg *config.VaultConfig, plan *workflow.Plan) error {
	// Validate everything first (best-effort). This ensures we fail before any writes.
	if _, err := previewPlan(vaultPath, vaultCfg, plan); err != nil {
		return handleError(ErrInvalidInput, err, "")
	}

	for i, op := range plan.Ops {
		if err := applyOp(vaultPath, vaultCfg, op); err != nil {
			return handleError(ErrInvalidInput, fmt.Errorf("op[%d] (%s): %w", i, op.Op, err), "")
		}
	}
	return nil
}

func applyOp(vaultPath string, vaultCfg *config.VaultConfig, op workflow.Op) error {
	switch op.Op {
	case "add":
		var args workflow.AddArgs
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return err
		}
		res, err := ResolveReference(args.To, ResolveOptions{VaultPath: vaultPath, VaultConfig: vaultCfg, AllowMissing: true})
		if err != nil {
			return err
		}
		if isProtectedAbs(vaultPath, res.FilePath, vaultCfg) {
			return fmt.Errorf("target is protected: %s", res.ObjectID)
		}

		// Configure capture formatting deterministically (no timestamps).
		captureCfg := vaultCfg.GetCaptureConfig()
		captureCfg.Timestamp = boolPtr(false)
		if args.Heading != "" {
			captureCfg.Heading = args.Heading
		} else {
			captureCfg.Heading = ""
		}

		// Only allow creating missing files for daily notes.
		allowCreate := false
		if _, err := os.Stat(res.FilePath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				allowCreate = isDailyObjectID(vaultCfg, res.FileObjectID)
			} else {
				return err
			}
		}

		line := args.Text
		if err := appendToFile(vaultPath, res.FilePath, line, captureCfg, vaultCfg, allowCreate, res.ObjectID); err != nil {
			return err
		}
		maybeReindex(vaultPath, res.FilePath, vaultCfg)
		return nil

	case "edit":
		var args workflow.EditArgs
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return err
		}
		abs := filepath.Join(vaultPath, args.Path)
		if err := paths.ValidateWithinVault(vaultPath, abs); err != nil {
			return err
		}
		if isProtectedAbs(vaultPath, abs, vaultCfg) {
			return fmt.Errorf("path is protected: %s", args.Path)
		}
		b, err := os.ReadFile(abs)
		if err != nil {
			return err
		}
		content := string(b)
		count := strings.Count(content, args.OldStr)
		if count != 1 {
			return fmt.Errorf("old_str must match exactly once (matches=%d)", count)
		}
		updated := strings.Replace(content, args.OldStr, args.NewStr, 1)
		if err := atomicfile.WriteFile(abs, []byte(updated), 0o644); err != nil {
			return err
		}
		maybeReindex(vaultPath, abs, vaultCfg)
		return nil

	case "set":
		var args workflow.SetArgs
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return err
		}
		res, err := ResolveReference(args.ObjectID, ResolveOptions{VaultPath: vaultPath, VaultConfig: vaultCfg})
		if err != nil {
			return err
		}
		if isProtectedAbs(vaultPath, res.FilePath, vaultCfg) {
			return fmt.Errorf("target is protected: %s", args.ObjectID)
		}
		return setSingleObject(vaultPath, res.ObjectID, args.Fields)

	case "move":
		var args workflow.MoveArgs
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return err
		}
		src, err := ResolveReference(args.Source, ResolveOptions{VaultPath: vaultPath, VaultConfig: vaultCfg})
		if err != nil {
			return err
		}
		if isProtectedAbs(vaultPath, src.FilePath, vaultCfg) {
			return fmt.Errorf("source is protected: %s", args.Source)
		}
		relDest := resolveDestinationRelPath(vaultCfg, args.Destination)
		if paths.IsProtectedRelPath(relDest, vaultCfg.ProtectedPrefixes) {
			return fmt.Errorf("destination is protected: %s", relDest)
		}
		return moveSingleObject(vaultPath, args.Source, args.Destination)

	case "update_trait":
		var args workflow.UpdateTraitArgs
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return err
		}
		return updateTraitByID(vaultPath, vaultCfg, args.TraitID, args.Value)

	default:
		return fmt.Errorf("unknown op: %s", op.Op)
	}
}

func printPlanPreview(name string, p *planPreview) {
	fmt.Printf("Workflow: %s\n", name)
	fmt.Println("Preview (use --confirm to apply):")
	for i, it := range p.Items {
		fmt.Printf("  %d. %s: %s\n", i+1, it.Op, it.Summary)
		if it.Why != "" {
			fmt.Printf("     why: %s\n", it.Why)
		}
	}
}

func isProtectedAbs(vaultPath string, absPath string, vaultCfg *config.VaultConfig) bool {
	rel, err := filepath.Rel(vaultPath, absPath)
	if err != nil {
		return true
	}
	rel = filepath.ToSlash(rel)
	extras := []string{}
	if vaultCfg != nil {
		extras = vaultCfg.ProtectedPrefixes
	}
	return paths.IsProtectedRelPath(rel, extras)
}

func resolveDestinationRelPath(vaultCfg *config.VaultConfig, destination string) string {
	dest := destination
	if strings.HasSuffix(dest, ".md") {
		return filepath.ToSlash(strings.TrimPrefix(dest, "./"))
	}
	if vaultCfg != nil {
		return filepath.ToSlash(vaultCfg.ResolveReferenceToFilePath(dest))
	}
	return filepath.ToSlash(dest + ".md")
}

func isDailyObjectID(vaultCfg *config.VaultConfig, objectID string) bool {
	if vaultCfg == nil {
		return strings.HasPrefix(objectID, "daily/")
	}
	return strings.HasPrefix(objectID, filepath.ToSlash(vaultCfg.DailyDirectory)+"/")
}

func parseTraitID(traitID string) (fileRel string, idx int, err error) {
	parts := strings.Split(traitID, ":trait:")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid trait id: %s", traitID)
	}
	fileRel = filepath.ToSlash(parts[0])
	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid trait index: %w", err)
	}
	return fileRel, n, nil
}

func updateTraitByID(vaultPath string, vaultCfg *config.VaultConfig, traitID string, newValue string) error {
	fileRel, idx, err := parseTraitID(traitID)
	if err != nil {
		return err
	}
	if paths.IsProtectedRelPath(fileRel, vaultCfg.ProtectedPrefixes) {
		return fmt.Errorf("trait file is protected: %s", fileRel)
	}

	abs := filepath.Join(vaultPath, fileRel)
	if err := paths.ValidateWithinVault(vaultPath, abs); err != nil {
		return err
	}

	b, err := os.ReadFile(abs)
	if err != nil {
		return err
	}

	lines := strings.Split(string(b), "\n")
	seen := 0
	updated := false

	for i := range lines {
		traits := parser.ParseTraitAnnotations(lines[i], i+1)
		if len(traits) == 0 {
			continue
		}
		for _, tr := range traits {
			if seen == idx {
				segment := lines[i][tr.StartOffset:tr.EndOffset]
				prefix := ""
				if len(segment) > 0 && segment[0] != '@' {
					prefix = segment[:1]
					segment = segment[1:]
				}
				// Replace with @name(newValue)
				repl := prefix + "@" + tr.TraitName + "(" + newValue + ")"
				lines[i] = lines[i][:tr.StartOffset] + repl + lines[i][tr.EndOffset:]
				updated = true
				break
			}
			seen++
		}
		if updated {
			break
		}
		// If we didn't update on this line, advance seen by number of traits on line.
		seen += 0
	}

	if !updated {
		return fmt.Errorf("trait not found: %s", traitID)
	}

	out := strings.Join(lines, "\n")
	if err := atomicfile.WriteFile(abs, []byte(out), 0o644); err != nil {
		return err
	}
	maybeReindex(vaultPath, abs, vaultCfg)
	return nil
}
