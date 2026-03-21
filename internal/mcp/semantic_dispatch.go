package mcp

import "github.com/aidanlsb/raven/internal/commands"

type semanticOp string

const (
	semanticObjectCreate          semanticOp = "object.create"
	semanticObjectUpsert          semanticOp = "object.upsert"
	semanticObjectAppend          semanticOp = "object.append"
	semanticObjectSetFields       semanticOp = "object.set_fields"
	semanticObjectDelete          semanticOp = "object.delete"
	semanticObjectMove            semanticOp = "object.move"
	semanticObjectReclassify      semanticOp = "object.reclassify"
	semanticObjectImport          semanticOp = "object.import"
	semanticObjectEdit            semanticOp = "object.edit"
	semanticTraitUpdate           semanticOp = "trait.update"
	semanticVaultInit             semanticOp = "vault.init"
	semanticVaultReindex          semanticOp = "vault.reindex"
	semanticVaultCheck            semanticOp = "vault.check"
	semanticVaultStats            semanticOp = "vault.stats"
	semanticVaultList             semanticOp = "vault.list"
	semanticVaultCurrent          semanticOp = "vault.current"
	semanticVaultUse              semanticOp = "vault.use"
	semanticVaultClear            semanticOp = "vault.clear"
	semanticVaultPin              semanticOp = "vault.pin"
	semanticVaultAdd              semanticOp = "vault.add"
	semanticVaultRemove           semanticOp = "vault.remove"
	semanticVaultPath             semanticOp = "vault.path"
	semanticConfigShow            semanticOp = "config.show"
	semanticConfigInit            semanticOp = "config.init"
	semanticConfigSet             semanticOp = "config.set"
	semanticConfigUnset           semanticOp = "config.unset"
	semanticDaily                 semanticOp = "date.daily"
	semanticDate                  semanticOp = "date.hub"
	semanticOpen                  semanticOp = "read.open"
	semanticQueryAdd              semanticOp = "query.add_saved"
	semanticQueryRemove           semanticOp = "query.remove_saved"
	semanticDocsBrowse            semanticOp = "docs.browse"
	semanticDocsFetch             semanticOp = "docs.fetch"
	semanticDocsList              semanticOp = "docs.list"
	semanticDocsSearch            semanticOp = "docs.search"
	semanticSkillList             semanticOp = "skill.list"
	semanticSkillInstall          semanticOp = "skill.install"
	semanticSkillRemove           semanticOp = "skill.remove"
	semanticSkillDoctor           semanticOp = "skill.doctor"
	semanticReadSearch            semanticOp = "read.search"
	semanticReadFile              semanticOp = "read.file"
	semanticReadBacklinks         semanticOp = "read.backlinks"
	semanticReadOutlinks          semanticOp = "read.outlinks"
	semanticReadResolve           semanticOp = "read.resolve"
	semanticReadQuery             semanticOp = "read.query"
	semanticSchemaAddType         semanticOp = "schema.add_type"
	semanticSchemaIntrospect      semanticOp = "schema.introspect"
	semanticSchemaAddTrait        semanticOp = "schema.add_trait"
	semanticSchemaAddField        semanticOp = "schema.add_field"
	semanticSchemaValidate        semanticOp = "schema.validate"
	semanticSchemaUpdateType      semanticOp = "schema.update_type"
	semanticSchemaUpdateTrait     semanticOp = "schema.update_trait"
	semanticSchemaUpdateField     semanticOp = "schema.update_field"
	semanticSchemaRemoveType      semanticOp = "schema.remove_type"
	semanticSchemaRemoveTrait     semanticOp = "schema.remove_trait"
	semanticSchemaRemoveField     semanticOp = "schema.remove_field"
	semanticSchemaRenameField     semanticOp = "schema.rename_field"
	semanticSchemaRenameType      semanticOp = "schema.rename_type"
	semanticSchemaTemplateList    semanticOp = "schema.template_list"
	semanticSchemaTemplateGet     semanticOp = "schema.template_get"
	semanticSchemaTemplateSet     semanticOp = "schema.template_set"
	semanticSchemaTemplateRemove  semanticOp = "schema.template_remove"
	semanticSchemaTemplateBind    semanticOp = "schema.template_bind"
	semanticSchemaTemplateUnbind  semanticOp = "schema.template_unbind"
	semanticSchemaTemplateDefault semanticOp = "schema.template_default"
	semanticTemplateList          semanticOp = "template.list"
	semanticTemplateWrite         semanticOp = "template.write"
	semanticTemplateDelete        semanticOp = "template.delete"
	semanticWorkflowList          semanticOp = "workflow.list"
	semanticWorkflowAdd           semanticOp = "workflow.add"
	semanticWorkflowScaffold      semanticOp = "workflow.scaffold"
	semanticWorkflowRemove        semanticOp = "workflow.remove"
	semanticWorkflowShow          semanticOp = "workflow.show"
	semanticWorkflowValidate      semanticOp = "workflow.validate"
	semanticWorkflowStepAdd       semanticOp = "workflow.step_add"
	semanticWorkflowStepUpdate    semanticOp = "workflow.step_update"
	semanticWorkflowStepRemove    semanticOp = "workflow.step_remove"
	semanticWorkflowRun           semanticOp = "workflow.run"
	semanticWorkflowContinue      semanticOp = "workflow.continue"
	semanticWorkflowRunsList      semanticOp = "workflow.runs_list"
	semanticWorkflowRunsStep      semanticOp = "workflow.runs_step"
	semanticWorkflowRunsPrune     semanticOp = "workflow.runs_prune"
	semanticSystemVersion         semanticOp = "system.version"
)

var compatibilityToolCommandAliases = map[string]string{
	"raven_vault":    "vault_list",
	"raven_config":   "config_show",
	"raven_template": "template_list",
}

func resolveCompatibilityCommandID(name string) (string, bool) {
	if commandID, ok := compatibilityToolCommandAliases[name]; ok {
		return commandID, true
	}
	return commands.ResolveToolCommandID(name)
}

func semanticOpForCommandID(commandID string) (semanticOp, bool) {
	switch commandID {
	case "new":
		return semanticObjectCreate, true
	case "upsert":
		return semanticObjectUpsert, true
	case "add":
		return semanticObjectAppend, true
	case "set":
		return semanticObjectSetFields, true
	case "delete":
		return semanticObjectDelete, true
	case "move":
		return semanticObjectMove, true
	case "reclassify":
		return semanticObjectReclassify, true
	case "import":
		return semanticObjectImport, true
	case "edit":
		return semanticObjectEdit, true
	case "update":
		return semanticTraitUpdate, true
	case "init":
		return semanticVaultInit, true
	case "reindex":
		return semanticVaultReindex, true
	case "check":
		return semanticVaultCheck, true
	case "vault_stats":
		return semanticVaultStats, true
	case "vault_list":
		return semanticVaultList, true
	case "vault_current":
		return semanticVaultCurrent, true
	case "vault_use":
		return semanticVaultUse, true
	case "vault_clear":
		return semanticVaultClear, true
	case "vault_pin":
		return semanticVaultPin, true
	case "vault_add":
		return semanticVaultAdd, true
	case "vault_remove":
		return semanticVaultRemove, true
	case "vault_path":
		return semanticVaultPath, true
	case "config_show":
		return semanticConfigShow, true
	case "config_init":
		return semanticConfigInit, true
	case "config_set":
		return semanticConfigSet, true
	case "config_unset":
		return semanticConfigUnset, true
	case "version":
		return semanticSystemVersion, true
	case "daily":
		return semanticDaily, true
	case "date":
		return semanticDate, true
	case "open":
		return semanticOpen, true
	case "query_add":
		return semanticQueryAdd, true
	case "query_remove":
		return semanticQueryRemove, true
	case "docs":
		return semanticDocsBrowse, true
	case "docs_fetch":
		return semanticDocsFetch, true
	case "docs_list":
		return semanticDocsList, true
	case "docs_search":
		return semanticDocsSearch, true
	case "skill_list":
		return semanticSkillList, true
	case "skill_install":
		return semanticSkillInstall, true
	case "skill_remove":
		return semanticSkillRemove, true
	case "skill_doctor":
		return semanticSkillDoctor, true
	case "search":
		return semanticReadSearch, true
	case "read":
		return semanticReadFile, true
	case "backlinks":
		return semanticReadBacklinks, true
	case "outlinks":
		return semanticReadOutlinks, true
	case "resolve":
		return semanticReadResolve, true
	case "query":
		return semanticReadQuery, true
	case "schema":
		return semanticSchemaIntrospect, true
	case "schema_add_type":
		return semanticSchemaAddType, true
	case "schema_add_trait":
		return semanticSchemaAddTrait, true
	case "schema_add_field":
		return semanticSchemaAddField, true
	case "schema_validate":
		return semanticSchemaValidate, true
	case "schema_update_type":
		return semanticSchemaUpdateType, true
	case "schema_update_trait":
		return semanticSchemaUpdateTrait, true
	case "schema_update_field":
		return semanticSchemaUpdateField, true
	case "schema_remove_type":
		return semanticSchemaRemoveType, true
	case "schema_remove_trait":
		return semanticSchemaRemoveTrait, true
	case "schema_remove_field":
		return semanticSchemaRemoveField, true
	case "schema_rename_field":
		return semanticSchemaRenameField, true
	case "schema_rename_type":
		return semanticSchemaRenameType, true
	case "schema_template_list":
		return semanticSchemaTemplateList, true
	case "schema_template_get":
		return semanticSchemaTemplateGet, true
	case "schema_template_set":
		return semanticSchemaTemplateSet, true
	case "schema_template_remove":
		return semanticSchemaTemplateRemove, true
	case "schema_template_bind":
		return semanticSchemaTemplateBind, true
	case "schema_template_unbind":
		return semanticSchemaTemplateUnbind, true
	case "schema_template_default":
		return semanticSchemaTemplateDefault, true
	case "template_list":
		return semanticTemplateList, true
	case "template_write":
		return semanticTemplateWrite, true
	case "template_delete":
		return semanticTemplateDelete, true
	case "workflow_list":
		return semanticWorkflowList, true
	case "workflow_add":
		return semanticWorkflowAdd, true
	case "workflow_scaffold":
		return semanticWorkflowScaffold, true
	case "workflow_remove":
		return semanticWorkflowRemove, true
	case "workflow_show":
		return semanticWorkflowShow, true
	case "workflow_validate":
		return semanticWorkflowValidate, true
	case "workflow_step_add":
		return semanticWorkflowStepAdd, true
	case "workflow_step_update":
		return semanticWorkflowStepUpdate, true
	case "workflow_step_remove":
		return semanticWorkflowStepRemove, true
	case "workflow_run":
		return semanticWorkflowRun, true
	case "workflow_continue":
		return semanticWorkflowContinue, true
	case "workflow_runs_list":
		return semanticWorkflowRunsList, true
	case "workflow_runs_step":
		return semanticWorkflowRunsStep, true
	case "workflow_runs_prune":
		return semanticWorkflowRunsPrune, true
	default:
		return "", false
	}
}

func semanticHandlerExists(op semanticOp) bool {
	switch op {
	case semanticObjectCreate,
		semanticObjectUpsert,
		semanticObjectAppend,
		semanticObjectSetFields,
		semanticObjectDelete,
		semanticObjectMove,
		semanticObjectReclassify,
		semanticObjectImport,
		semanticObjectEdit,
		semanticTraitUpdate,
		semanticVaultInit,
		semanticVaultReindex,
		semanticVaultCheck,
		semanticVaultStats,
		semanticVaultList,
		semanticVaultCurrent,
		semanticVaultUse,
		semanticVaultClear,
		semanticVaultPin,
		semanticVaultAdd,
		semanticVaultRemove,
		semanticVaultPath,
		semanticConfigShow,
		semanticConfigInit,
		semanticConfigSet,
		semanticConfigUnset,
		semanticDaily,
		semanticDate,
		semanticOpen,
		semanticQueryAdd,
		semanticQueryRemove,
		semanticDocsBrowse,
		semanticDocsFetch,
		semanticDocsList,
		semanticDocsSearch,
		semanticSkillList,
		semanticSkillInstall,
		semanticSkillRemove,
		semanticSkillDoctor,
		semanticReadSearch,
		semanticReadFile,
		semanticReadBacklinks,
		semanticReadOutlinks,
		semanticReadResolve,
		semanticReadQuery,
		semanticSchemaIntrospect,
		semanticSchemaAddType,
		semanticSchemaAddTrait,
		semanticSchemaAddField,
		semanticSchemaValidate,
		semanticSchemaUpdateType,
		semanticSchemaUpdateTrait,
		semanticSchemaUpdateField,
		semanticSchemaRemoveType,
		semanticSchemaRemoveTrait,
		semanticSchemaRemoveField,
		semanticSchemaRenameField,
		semanticSchemaRenameType,
		semanticSchemaTemplateList,
		semanticSchemaTemplateGet,
		semanticSchemaTemplateSet,
		semanticSchemaTemplateRemove,
		semanticSchemaTemplateBind,
		semanticSchemaTemplateUnbind,
		semanticSchemaTemplateDefault,
		semanticTemplateList,
		semanticTemplateWrite,
		semanticTemplateDelete,
		semanticWorkflowList,
		semanticWorkflowAdd,
		semanticWorkflowScaffold,
		semanticWorkflowRemove,
		semanticWorkflowShow,
		semanticWorkflowValidate,
		semanticWorkflowStepAdd,
		semanticWorkflowStepUpdate,
		semanticWorkflowStepRemove,
		semanticWorkflowRun,
		semanticWorkflowContinue,
		semanticWorkflowRunsList,
		semanticWorkflowRunsStep,
		semanticWorkflowRunsPrune,
		semanticSystemVersion:
		return true
	default:
		return false
	}
}

func (s *Server) callSemanticTool(op semanticOp, args map[string]interface{}) (string, bool, bool) {
	switch op {
	case semanticObjectCreate:
		out, isErr := s.callDirectNew(args)
		return out, isErr, true
	case semanticObjectUpsert:
		out, isErr := s.callDirectUpsert(args)
		return out, isErr, true
	case semanticObjectAppend:
		out, isErr := s.callDirectAdd(args)
		return out, isErr, true
	case semanticObjectSetFields:
		out, isErr := s.callDirectSet(args)
		return out, isErr, true
	case semanticObjectDelete:
		out, isErr := s.callDirectDelete(args)
		return out, isErr, true
	case semanticObjectMove:
		out, isErr := s.callDirectMove(args)
		return out, isErr, true
	case semanticObjectReclassify:
		out, isErr := s.callDirectReclassify(args)
		return out, isErr, true
	case semanticObjectImport:
		out, isErr := s.callDirectImport(args)
		return out, isErr, true
	case semanticObjectEdit:
		out, isErr := s.callDirectEdit(args)
		return out, isErr, true
	case semanticTraitUpdate:
		out, isErr := s.callDirectUpdate(args)
		return out, isErr, true
	case semanticVaultInit:
		out, isErr := s.callDirectInit(args)
		return out, isErr, true
	case semanticVaultReindex:
		out, isErr := s.callDirectReindex(args)
		return out, isErr, true
	case semanticVaultCheck:
		out, isErr := s.callDirectCheck(args)
		return out, isErr, true
	case semanticVaultStats:
		out, isErr := s.callDirectStats(args)
		return out, isErr, true
	case semanticVaultList:
		out, isErr := s.callDirectVaultList(args)
		return out, isErr, true
	case semanticVaultCurrent:
		out, isErr := s.callDirectVaultCurrent(args)
		return out, isErr, true
	case semanticVaultUse:
		out, isErr := s.callDirectVaultUse(args)
		return out, isErr, true
	case semanticVaultClear:
		out, isErr := s.callDirectVaultClear(args)
		return out, isErr, true
	case semanticVaultPin:
		out, isErr := s.callDirectVaultPin(args)
		return out, isErr, true
	case semanticVaultAdd:
		out, isErr := s.callDirectVaultAdd(args)
		return out, isErr, true
	case semanticVaultRemove:
		out, isErr := s.callDirectVaultRemove(args)
		return out, isErr, true
	case semanticVaultPath:
		out, isErr := s.callDirectVaultPath(args)
		return out, isErr, true
	case semanticConfigShow:
		out, isErr := s.callDirectConfigShow(args)
		return out, isErr, true
	case semanticConfigInit:
		out, isErr := s.callDirectConfigInit(args)
		return out, isErr, true
	case semanticConfigSet:
		out, isErr := s.callDirectConfigSet(args)
		return out, isErr, true
	case semanticConfigUnset:
		out, isErr := s.callDirectConfigUnset(args)
		return out, isErr, true
	case semanticSystemVersion:
		out, isErr := s.callDirectVersion(args)
		return out, isErr, true
	case semanticDaily:
		out, isErr := s.callDirectDaily(args)
		return out, isErr, true
	case semanticDate:
		out, isErr := s.callDirectDate(args)
		return out, isErr, true
	case semanticOpen:
		out, isErr := s.callDirectOpen(args)
		return out, isErr, true
	case semanticQueryAdd:
		out, isErr := s.callDirectQueryAdd(args)
		return out, isErr, true
	case semanticQueryRemove:
		out, isErr := s.callDirectQueryRemove(args)
		return out, isErr, true
	case semanticDocsBrowse:
		out, isErr := s.callDirectDocs(args)
		return out, isErr, true
	case semanticDocsFetch:
		out, isErr := s.callDirectDocsFetch(args)
		return out, isErr, true
	case semanticDocsList:
		out, isErr := s.callDirectDocsList(args)
		return out, isErr, true
	case semanticDocsSearch:
		out, isErr := s.callDirectDocsSearch(args)
		return out, isErr, true
	case semanticSkillList:
		out, isErr := s.callDirectSkillList(args)
		return out, isErr, true
	case semanticSkillInstall:
		out, isErr := s.callDirectSkillInstall(args)
		return out, isErr, true
	case semanticSkillRemove:
		out, isErr := s.callDirectSkillRemove(args)
		return out, isErr, true
	case semanticSkillDoctor:
		out, isErr := s.callDirectSkillDoctor(args)
		return out, isErr, true
	case semanticReadSearch:
		out, isErr := s.callDirectSearch(args)
		return out, isErr, true
	case semanticReadFile:
		out, isErr := s.callDirectRead(args)
		return out, isErr, true
	case semanticReadBacklinks:
		out, isErr := s.callDirectBacklinks(args)
		return out, isErr, true
	case semanticReadOutlinks:
		out, isErr := s.callDirectOutlinks(args)
		return out, isErr, true
	case semanticReadResolve:
		out, isErr := s.callDirectResolve(args)
		return out, isErr, true
	case semanticReadQuery:
		out, isErr := s.callDirectQuery(args)
		return out, isErr, true
	case semanticSchemaIntrospect:
		out, isErr := s.callDirectSchema(args)
		return out, isErr, true
	case semanticSchemaAddType:
		out, isErr := s.callDirectSchemaAddType(args)
		return out, isErr, true
	case semanticSchemaAddTrait:
		out, isErr := s.callDirectSchemaAddTrait(args)
		return out, isErr, true
	case semanticSchemaAddField:
		out, isErr := s.callDirectSchemaAddField(args)
		return out, isErr, true
	case semanticSchemaValidate:
		out, isErr := s.callDirectSchemaValidate(args)
		return out, isErr, true
	case semanticSchemaUpdateType:
		out, isErr := s.callDirectSchemaUpdateType(args)
		return out, isErr, true
	case semanticSchemaUpdateTrait:
		out, isErr := s.callDirectSchemaUpdateTrait(args)
		return out, isErr, true
	case semanticSchemaUpdateField:
		out, isErr := s.callDirectSchemaUpdateField(args)
		return out, isErr, true
	case semanticSchemaRemoveType:
		out, isErr := s.callDirectSchemaRemoveType(args)
		return out, isErr, true
	case semanticSchemaRemoveTrait:
		out, isErr := s.callDirectSchemaRemoveTrait(args)
		return out, isErr, true
	case semanticSchemaRemoveField:
		out, isErr := s.callDirectSchemaRemoveField(args)
		return out, isErr, true
	case semanticSchemaRenameField:
		out, isErr := s.callDirectSchemaRenameField(args)
		return out, isErr, true
	case semanticSchemaRenameType:
		out, isErr := s.callDirectSchemaRenameType(args)
		return out, isErr, true
	case semanticSchemaTemplateList:
		out, isErr := s.callDirectSchemaTemplateList(args)
		return out, isErr, true
	case semanticSchemaTemplateGet:
		out, isErr := s.callDirectSchemaTemplateGet(args)
		return out, isErr, true
	case semanticSchemaTemplateSet:
		out, isErr := s.callDirectSchemaTemplateSet(args)
		return out, isErr, true
	case semanticSchemaTemplateRemove:
		out, isErr := s.callDirectSchemaTemplateRemove(args)
		return out, isErr, true
	case semanticSchemaTemplateBind:
		out, isErr := s.callDirectSchemaTemplateBind(args)
		return out, isErr, true
	case semanticSchemaTemplateUnbind:
		out, isErr := s.callDirectSchemaTemplateUnbind(args)
		return out, isErr, true
	case semanticSchemaTemplateDefault:
		out, isErr := s.callDirectSchemaTemplateDefault(args)
		return out, isErr, true
	case semanticTemplateList:
		out, isErr := s.callDirectTemplateList(args)
		return out, isErr, true
	case semanticTemplateWrite:
		out, isErr := s.callDirectTemplateWrite(args)
		return out, isErr, true
	case semanticTemplateDelete:
		out, isErr := s.callDirectTemplateDelete(args)
		return out, isErr, true
	case semanticWorkflowList:
		out, isErr := s.callDirectWorkflowList(args)
		return out, isErr, true
	case semanticWorkflowAdd:
		out, isErr := s.callDirectWorkflowAdd(args)
		return out, isErr, true
	case semanticWorkflowScaffold:
		out, isErr := s.callDirectWorkflowScaffold(args)
		return out, isErr, true
	case semanticWorkflowRemove:
		out, isErr := s.callDirectWorkflowRemove(args)
		return out, isErr, true
	case semanticWorkflowShow:
		out, isErr := s.callDirectWorkflowShow(args)
		return out, isErr, true
	case semanticWorkflowValidate:
		out, isErr := s.callDirectWorkflowValidate(args)
		return out, isErr, true
	case semanticWorkflowStepAdd:
		out, isErr := s.callDirectWorkflowStepAdd(args)
		return out, isErr, true
	case semanticWorkflowStepUpdate:
		out, isErr := s.callDirectWorkflowStepUpdate(args)
		return out, isErr, true
	case semanticWorkflowStepRemove:
		out, isErr := s.callDirectWorkflowStepRemove(args)
		return out, isErr, true
	case semanticWorkflowRun:
		out, isErr := s.callDirectWorkflowRun(args)
		return out, isErr, true
	case semanticWorkflowContinue:
		out, isErr := s.callDirectWorkflowContinue(args)
		return out, isErr, true
	case semanticWorkflowRunsList:
		out, isErr := s.callDirectWorkflowRunsList(args)
		return out, isErr, true
	case semanticWorkflowRunsStep:
		out, isErr := s.callDirectWorkflowRunsStep(args)
		return out, isErr, true
	case semanticWorkflowRunsPrune:
		out, isErr := s.callDirectWorkflowRunsPrune(args)
		return out, isErr, true
	default:
		return "", false, false
	}
}

func (s *Server) callToolDirect(name string, args map[string]interface{}) (string, bool, bool) {
	commandID, ok := resolveCompatibilityCommandID(name)
	if !ok {
		return "", false, false
	}

	op, ok := semanticOpForCommandID(commandID)
	if !ok {
		return "", false, false
	}

	out, isErr, handled := s.callSemanticTool(op, args)
	if !handled {
		return errorEnvelope(
			"INTERNAL_ERROR",
			"semantic handler is not configured",
			"report this issue with the failing command id and semantic operation",
			map[string]interface{}{"tool_name": name, "command": commandID, "semantic_op": op},
		), true, true
	}
	return out, isErr, true
}
