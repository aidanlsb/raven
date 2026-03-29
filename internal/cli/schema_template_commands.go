package cli

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/schemasvc"
)

type schemaTemplateTarget struct {
	kind string
	name string
}

var schemaTemplateCmd = &cobra.Command{
	Use:   "template",
	Short: "Manage schema templates and bindings",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var schemaTemplateListCmd = newCanonicalLeafCommand("schema_template_list", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	BuildArgs:   buildSchemaTemplateListArgs,
	RenderHuman: renderSchemaTemplateList,
})

var schemaTemplateGetCmd = newCanonicalLeafCommand("schema_template_get", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderSchemaTemplateGet,
})

var schemaTemplateSetCmd = newCanonicalLeafCommand("schema_template_set", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderSchemaTemplateSet,
})

var schemaTemplateRemoveCmd = newCanonicalLeafCommand("schema_template_remove", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderSchemaTemplateRemove,
})

var schemaTemplateBindCmd = newCanonicalLeafCommand("schema_template_bind", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	BuildArgs:   buildSchemaTemplateBindArgs,
	RenderHuman: renderSchemaTemplateBind,
})

var schemaTemplateUnbindCmd = newCanonicalLeafCommand("schema_template_unbind", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	BuildArgs:   buildSchemaTemplateUnbindArgs,
	RenderHuman: renderSchemaTemplateUnbind,
})

var schemaTemplateDefaultCmd = newCanonicalLeafCommand("schema_template_default", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	BuildArgs:   buildSchemaTemplateDefaultArgs,
	RenderHuman: renderSchemaTemplateDefault,
})

func resolveSchemaTemplateTarget(cmd *cobra.Command, required bool) (*schemaTemplateTarget, error) {
	typeName, _ := cmd.Flags().GetString("type")
	coreType, _ := cmd.Flags().GetString("core")
	typeName = strings.TrimSpace(typeName)
	coreType = strings.TrimSpace(coreType)

	switch {
	case typeName != "" && coreType != "":
		return nil, handleErrorMsg(ErrInvalidInput, "specify exactly one of --type or --core", "")
	case typeName != "":
		return &schemaTemplateTarget{kind: "type", name: typeName}, nil
	case coreType != "":
		return &schemaTemplateTarget{kind: "core", name: coreType}, nil
	case required:
		return nil, handleErrorMsg(ErrMissingArgument, "specify --type or --core", "")
	default:
		return nil, nil
	}
}

func buildSchemaTemplateListArgs(cmd *cobra.Command, _ []string) (map[string]interface{}, error) {
	target, err := resolveSchemaTemplateTarget(cmd, false)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, nil
	}
	return map[string]interface{}{target.kind: target.name}, nil
}

func renderSchemaTemplateList(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	if _, ok := data["default_template"]; ok || data["type"] != nil || data["core"] != nil {
		return renderSchemaTemplateBindings(result)
	}
	return renderSchemaTemplateDefinitions(result)
}

func renderSchemaTemplateDefinitions(result commandexec.Result) error {
	data := canonicalDataMap(result)
	items, err := decodeSchemaValue[[]schemasvc.TemplateSchema](data["templates"])
	if err != nil {
		return err
	}

	if len(items) == 0 {
		fmt.Println("No schema templates configured.")
		return nil
	}
	fmt.Println("Schema templates:")
	for _, it := range items {
		if it.Description != "" {
			fmt.Printf("  %s -> %s (%s)\n", it.ID, it.File, it.Description)
		} else {
			fmt.Printf("  %s -> %s\n", it.ID, it.File)
		}
	}
	return nil
}

func renderSchemaTemplateBindings(result commandexec.Result) error {
	data := canonicalDataMap(result)
	templates, err := decodeSchemaValue[[]string](data["templates"])
	if err != nil {
		return err
	}
	defaultTemplate, _ := data["default_template"].(string)
	kind := strings.TrimSpace(stringValue(data["type"]))
	if kind != "" {
		return renderSchemaTemplateBindingTarget("type", kind, templates, defaultTemplate)
	}
	return renderSchemaTemplateBindingTarget("core", strings.TrimSpace(stringValue(data["core"])), templates, defaultTemplate)
}

func renderSchemaTemplateBindingTarget(kind, name string, templates []string, defaultTemplate string) error {
	label := schemaTemplateKindLabel(kind)
	fmt.Printf("%s templates for %s:\n", label, name)
	if len(templates) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, templateID := range templates {
			fmt.Printf("  - %s\n", templateID)
		}
	}
	if defaultTemplate != "" {
		fmt.Printf("Default: %s\n", defaultTemplate)
	} else {
		fmt.Println("Default: (none)")
	}
	return nil
}

func renderSchemaTemplateGet(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	id, _ := data["id"].(string)
	file, _ := data["file"].(string)
	description, _ := data["description"].(string)
	fmt.Printf("Template: %s\n", id)
	fmt.Printf("  File: %s\n", file)
	if description != "" {
		fmt.Printf("  Description: %s\n", description)
	}
	return nil
}

func renderSchemaTemplateSet(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Printf("Set schema template %s -> %s\n", data["id"], data["file"])
	return nil
}

func renderSchemaTemplateRemove(_ *cobra.Command, result commandexec.Result) error {
	fmt.Printf("Removed schema template %s\n", strings.TrimSpace(stringValue(canonicalDataMap(result)["id"])))
	return nil
}

func buildSchemaTemplateBindArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	target, err := resolveSchemaTemplateTarget(cmd, true)
	if err != nil {
		return nil, err
	}
	argsMap := map[string]interface{}{"template_id": args[0], target.kind: target.name}
	if cmd.Flags().Changed("default") {
		value, _ := cmd.Flags().GetBool("default")
		argsMap["default"] = value
	}
	return argsMap, nil
}

func renderSchemaTemplateBind(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	kind, name := schemaTemplateResultTarget(data)
	templateID := strings.TrimSpace(stringValue(data["template_id"]))
	setDefault := strings.TrimSpace(stringValue(data["default_template"])) != ""
	if boolValue(data["already_set"]) {
		fmt.Printf("%s %s already includes template %s\n", schemaTemplateKindLabel(kind), name, templateID)
		if setDefault {
			fmt.Printf("Set default template for %s %s -> %s\n", kind, name, templateID)
		}
		return nil
	}

	fmt.Printf("Bound template %s to %s %s\n", templateID, kind, name)
	if setDefault {
		fmt.Printf("Set default template for %s %s -> %s\n", kind, name, templateID)
	}
	return nil
}

func buildSchemaTemplateUnbindArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	target, err := resolveSchemaTemplateTarget(cmd, true)
	if err != nil {
		return nil, err
	}
	argsMap := map[string]interface{}{"template_id": args[0], target.kind: target.name}
	if cmd.Flags().Changed("clear-default") {
		value, _ := cmd.Flags().GetBool("clear-default")
		argsMap["clear-default"] = value
	}
	return argsMap, nil
}

func renderSchemaTemplateUnbind(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	kind, name := schemaTemplateResultTarget(data)
	templateID := strings.TrimSpace(stringValue(data["template_id"]))
	clearDefault := boolValue(data["default_cleared"])
	if clearDefault {
		fmt.Printf("Cleared default template and unbound %s from %s %s\n", templateID, kind, name)
		return nil
	}
	fmt.Printf("Unbound template %s from %s %s\n", templateID, kind, name)
	return nil
}

func buildSchemaTemplateDefaultArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	target, err := resolveSchemaTemplateTarget(cmd, true)
	if err != nil {
		return nil, err
	}
	argsMap := map[string]interface{}{target.kind: target.name}
	if len(args) > 0 {
		argsMap["template_id"] = args[0]
	}
	if cmd.Flags().Changed("clear") {
		value, _ := cmd.Flags().GetBool("clear")
		argsMap["clear"] = value
	}
	return argsMap, nil
}

func renderSchemaTemplateDefault(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	kind, name := schemaTemplateResultTarget(data)
	newDefault, _ := data["default_template"].(string)
	if strings.TrimSpace(newDefault) == "" {
		fmt.Printf("Cleared default template for %s %s\n", kind, name)
		return nil
	}
	fmt.Printf("Set default template for %s %s -> %s\n", kind, name, newDefault)
	return nil
}

func schemaTemplateKindLabel(kind string) string {
	if kind == "" {
		return kind
	}
	runes := []rune(kind)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func schemaTemplateResultTarget(data map[string]interface{}) (kind string, name string) {
	if typeName := strings.TrimSpace(stringValue(data["type"])); typeName != "" {
		return "type", typeName
	}
	return "core", strings.TrimSpace(stringValue(data["core"]))
}

func init() {
	schemaTemplateCmd.AddCommand(schemaTemplateListCmd)
	schemaTemplateCmd.AddCommand(schemaTemplateGetCmd)
	schemaTemplateCmd.AddCommand(schemaTemplateSetCmd)
	schemaTemplateCmd.AddCommand(schemaTemplateRemoveCmd)
	schemaTemplateCmd.AddCommand(schemaTemplateBindCmd)
	schemaTemplateCmd.AddCommand(schemaTemplateUnbindCmd)
	schemaTemplateCmd.AddCommand(schemaTemplateDefaultCmd)
	schemaCmd.AddCommand(schemaTemplateCmd)
}
