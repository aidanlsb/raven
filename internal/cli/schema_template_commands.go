package cli

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/schemasvc"
	"github.com/aidanlsb/raven/internal/ui"
)

type schemaTemplateTarget struct {
	kind string
	name string
}

// schemaTemplateCmd is the "schema template" subtree. Its command hierarchy is
// generated from registry metadata (CLIPath) via buildRegistrySubtree, so
// adding a new schema_template_* registry entry only requires registering its
// human RenderHuman hook below — no new hand-written Cobra vars or AddCommand wiring.
var schemaTemplateCmd = buildSchemaTemplateCommand()

func buildSchemaTemplateCommand() *cobra.Command {
	return buildRegistrySubtree(registrySubtreeSpec{
		Prefix:    []string{"schema", "template"},
		VaultPath: getVaultPath,
		Root: registryGroup{
			Use:   "template",
			Short: "Manage schema templates and bindings",
		},
		Renders: map[string]func(*cobra.Command, commandexec.Result) error{
			"schema_template_list":   renderSchemaTemplateList,
			"schema_template_get":    renderSchemaTemplateGet,
			"schema_template_set":    renderSchemaTemplateSet,
			"schema_template_remove": renderSchemaTemplateRemove,
			"schema_template_bind":   renderSchemaTemplateBind,
			"schema_template_unbind": renderSchemaTemplateUnbind,
		},
		Leaves: map[string]canonicalLeafOptions{
			"schema_template_list":   {Invoke: invokeSchemaTemplateList},
			"schema_template_bind":   {Invoke: invokeSchemaTemplateBind},
			"schema_template_unbind": {Invoke: invokeSchemaTemplateUnbind},
		},
	})
}

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

func invokeSchemaTemplateList(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	target, err := resolveSchemaTemplateTarget(cmd, false)
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "")
	}
	if target != nil {
		args[target.kind] = target.name
	}
	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
	})
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
		fmt.Println(ui.Star("No schema templates configured."))
		return nil
	}
	fmt.Println(ui.SectionHeader("Schema templates"))
	for _, it := range items {
		if it.Description != "" {
			fmt.Println(ui.Bullet(fmt.Sprintf("%s -> %s %s", it.ID, ui.FilePath(it.File), ui.Hint("("+it.Description+")"))))
		} else {
			fmt.Println(ui.Bullet(fmt.Sprintf("%s -> %s", it.ID, ui.FilePath(it.File))))
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
	fmt.Println(ui.SectionHeader(fmt.Sprintf("%s templates for %s", label, name)))
	if len(templates) == 0 {
		fmt.Println(ui.Bullet(ui.Hint("(none)")))
	} else {
		for _, templateID := range templates {
			fmt.Println(ui.Bullet(templateID))
		}
	}
	if defaultTemplate != "" {
		fmt.Printf("%s %s\n", ui.Hint("Default:"), defaultTemplate)
	} else {
		fmt.Printf("%s %s\n", ui.Hint("Default:"), ui.Hint("(none)"))
	}
	return nil
}

func renderSchemaTemplateGet(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.SchemaTemplateDefinitionResult](result)
	if err != nil {
		return err
	}
	fmt.Printf("%s %s\n", ui.Hint("Template:"), data.ID)
	fmt.Printf("  %s %s\n", ui.Hint("File:"), ui.FilePath(data.File))
	if data.Description != "" {
		fmt.Printf("  %s %s\n", ui.Hint("Description:"), data.Description)
	}
	return nil
}

func renderSchemaTemplateSet(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.SchemaTemplateDefinitionResult](result)
	if err != nil {
		return err
	}
	fmt.Println(ui.Checkf("Set schema template %s -> %s", data.ID, ui.FilePath(data.File)))
	return nil
}

func renderSchemaTemplateRemove(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.SchemaTemplateRemoveResult](result)
	if err != nil {
		return err
	}
	fmt.Println(ui.Checkf("Removed schema template %s", strings.TrimSpace(data.ID)))
	return nil
}

func invokeSchemaTemplateBind(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	target, err := resolveSchemaTemplateTarget(cmd, true)
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "")
	}
	args[target.kind] = target.name
	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
	})
}

func renderSchemaTemplateBind(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.SchemaTemplateBindResult](result)
	if err != nil {
		return err
	}
	kind, name := schemaTemplateResultTarget(data.Type, data.Core)
	templateID := strings.TrimSpace(data.TemplateID)
	setDefault := strings.TrimSpace(data.DefaultTemplate) != ""
	if data.AlreadySet {
		fmt.Println(ui.Starf("%s %s already includes template %s", schemaTemplateKindLabel(kind), name, templateID))
		if setDefault {
			fmt.Println(ui.Checkf("Set default template for %s %s -> %s", kind, name, templateID))
		}
		return nil
	}

	fmt.Println(ui.Checkf("Bound template %s to %s %s", templateID, kind, name))
	if setDefault {
		fmt.Println(ui.Checkf("Set default template for %s %s -> %s", kind, name, templateID))
	}
	return nil
}

func invokeSchemaTemplateUnbind(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	target, err := resolveSchemaTemplateTarget(cmd, true)
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "")
	}
	args[target.kind] = target.name
	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
	})
}

func renderSchemaTemplateUnbind(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.SchemaTemplateUnbindResult](result)
	if err != nil {
		return err
	}
	kind, name := schemaTemplateResultTarget(data.Type, data.Core)
	templateID := strings.TrimSpace(data.TemplateID)
	if data.DefaultCleared {
		fmt.Println(ui.Checkf("Cleared default template and unbound %s from %s %s", templateID, kind, name))
		return nil
	}
	fmt.Println(ui.Checkf("Unbound template %s from %s %s", templateID, kind, name))
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

func schemaTemplateResultTarget(typeName, coreType string) (kind string, name string) {
	if typeName = strings.TrimSpace(typeName); typeName != "" {
		return "type", typeName
	}
	return "core", strings.TrimSpace(coreType)
}

func init() {
	schemaCmd.AddCommand(schemaTemplateCmd)
}
