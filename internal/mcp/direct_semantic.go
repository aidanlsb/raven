package mcp

type semanticOp string

const (
	semanticObjectCreate              semanticOp = "object.create"
	semanticObjectUpsert              semanticOp = "object.upsert"
	semanticObjectAppend              semanticOp = "object.append"
	semanticObjectSetFields           semanticOp = "object.set_fields"
	semanticObjectDelete              semanticOp = "object.delete"
	semanticObjectMove                semanticOp = "object.move"
	semanticVaultReindex              semanticOp = "vault.reindex"
	semanticVaultCheck                semanticOp = "vault.check"
	semanticReadSearch                semanticOp = "read.search"
	semanticReadFile                  semanticOp = "read.file"
	semanticReadBacklinks             semanticOp = "read.backlinks"
	semanticReadOutlinks              semanticOp = "read.outlinks"
	semanticReadResolve               semanticOp = "read.resolve"
	semanticReadQuery                 semanticOp = "read.query"
	semanticSchemaAddType             semanticOp = "schema.add_type"
	semanticSchemaAddTrait            semanticOp = "schema.add_trait"
	semanticSchemaAddField            semanticOp = "schema.add_field"
	semanticSchemaValidate            semanticOp = "schema.validate"
	semanticSchemaUpdateType          semanticOp = "schema.update_type"
	semanticSchemaUpdateTrait         semanticOp = "schema.update_trait"
	semanticSchemaUpdateField         semanticOp = "schema.update_field"
	semanticSchemaRemoveType          semanticOp = "schema.remove_type"
	semanticSchemaRemoveTrait         semanticOp = "schema.remove_trait"
	semanticSchemaRemoveField         semanticOp = "schema.remove_field"
	semanticSchemaRenameField         semanticOp = "schema.rename_field"
	semanticSchemaRenameType          semanticOp = "schema.rename_type"
	semanticSchemaTemplateList        semanticOp = "schema.template_list"
	semanticSchemaTemplateGet         semanticOp = "schema.template_get"
	semanticSchemaTemplateSet         semanticOp = "schema.template_set"
	semanticSchemaTemplateRemove      semanticOp = "schema.template_remove"
	semanticSchemaTypeTemplateList    semanticOp = "schema.type_template_list"
	semanticSchemaTypeTemplateSet     semanticOp = "schema.type_template_set"
	semanticSchemaTypeTemplateRemove  semanticOp = "schema.type_template_remove"
	semanticSchemaTypeTemplateDefault semanticOp = "schema.type_template_default"
	semanticSchemaCoreTemplateList    semanticOp = "schema.core_template_list"
	semanticSchemaCoreTemplateSet     semanticOp = "schema.core_template_set"
	semanticSchemaCoreTemplateRemove  semanticOp = "schema.core_template_remove"
	semanticSchemaCoreTemplateDefault semanticOp = "schema.core_template_default"
	semanticWorkflowList              semanticOp = "workflow.list"
	semanticWorkflowAdd               semanticOp = "workflow.add"
	semanticWorkflowScaffold          semanticOp = "workflow.scaffold"
	semanticWorkflowRemove            semanticOp = "workflow.remove"
	semanticWorkflowShow              semanticOp = "workflow.show"
	semanticWorkflowValidate          semanticOp = "workflow.validate"
	semanticWorkflowStepAdd           semanticOp = "workflow.step_add"
	semanticWorkflowStepUpdate        semanticOp = "workflow.step_update"
	semanticWorkflowStepRemove        semanticOp = "workflow.step_remove"
	semanticWorkflowRun               semanticOp = "workflow.run"
	semanticWorkflowContinue          semanticOp = "workflow.continue"
	semanticWorkflowRunsList          semanticOp = "workflow.runs_list"
	semanticWorkflowRunsStep          semanticOp = "workflow.runs_step"
	semanticWorkflowRunsPrune         semanticOp = "workflow.runs_prune"
)

var compatibilityToolSemanticMap = map[string]semanticOp{
	"raven_new":                          semanticObjectCreate,
	"raven_upsert":                       semanticObjectUpsert,
	"raven_add":                          semanticObjectAppend,
	"raven_set":                          semanticObjectSetFields,
	"raven_delete":                       semanticObjectDelete,
	"raven_move":                         semanticObjectMove,
	"raven_reindex":                      semanticVaultReindex,
	"raven_check":                        semanticVaultCheck,
	"raven_search":                       semanticReadSearch,
	"raven_read":                         semanticReadFile,
	"raven_backlinks":                    semanticReadBacklinks,
	"raven_outlinks":                     semanticReadOutlinks,
	"raven_resolve":                      semanticReadResolve,
	"raven_query":                        semanticReadQuery,
	"raven_schema_add_type":              semanticSchemaAddType,
	"raven_schema_add_trait":             semanticSchemaAddTrait,
	"raven_schema_add_field":             semanticSchemaAddField,
	"raven_schema_validate":              semanticSchemaValidate,
	"raven_schema_update_type":           semanticSchemaUpdateType,
	"raven_schema_update_trait":          semanticSchemaUpdateTrait,
	"raven_schema_update_field":          semanticSchemaUpdateField,
	"raven_schema_remove_type":           semanticSchemaRemoveType,
	"raven_schema_remove_trait":          semanticSchemaRemoveTrait,
	"raven_schema_remove_field":          semanticSchemaRemoveField,
	"raven_schema_rename_field":          semanticSchemaRenameField,
	"raven_schema_rename_type":           semanticSchemaRenameType,
	"raven_schema_template_list":         semanticSchemaTemplateList,
	"raven_schema_template_get":          semanticSchemaTemplateGet,
	"raven_schema_template_set":          semanticSchemaTemplateSet,
	"raven_schema_template_remove":       semanticSchemaTemplateRemove,
	"raven_schema_type_template_list":    semanticSchemaTypeTemplateList,
	"raven_schema_type_template_set":     semanticSchemaTypeTemplateSet,
	"raven_schema_type_template_remove":  semanticSchemaTypeTemplateRemove,
	"raven_schema_type_template_default": semanticSchemaTypeTemplateDefault,
	"raven_schema_core_template_list":    semanticSchemaCoreTemplateList,
	"raven_schema_core_template_set":     semanticSchemaCoreTemplateSet,
	"raven_schema_core_template_remove":  semanticSchemaCoreTemplateRemove,
	"raven_schema_core_template_default": semanticSchemaCoreTemplateDefault,
	"raven_workflow_list":                semanticWorkflowList,
	"raven_workflow_add":                 semanticWorkflowAdd,
	"raven_workflow_scaffold":            semanticWorkflowScaffold,
	"raven_workflow_remove":              semanticWorkflowRemove,
	"raven_workflow_show":                semanticWorkflowShow,
	"raven_workflow_validate":            semanticWorkflowValidate,
	"raven_workflow_step_add":            semanticWorkflowStepAdd,
	"raven_workflow_step_update":         semanticWorkflowStepUpdate,
	"raven_workflow_step_remove":         semanticWorkflowStepRemove,
	"raven_workflow_run":                 semanticWorkflowRun,
	"raven_workflow_continue":            semanticWorkflowContinue,
	"raven_workflow_runs_list":           semanticWorkflowRunsList,
	"raven_workflow_runs_step":           semanticWorkflowRunsStep,
	"raven_workflow_runs_prune":          semanticWorkflowRunsPrune,
}

func semanticHandlerExists(op semanticOp) bool {
	switch op {
	case semanticObjectCreate,
		semanticObjectUpsert,
		semanticObjectAppend,
		semanticObjectSetFields,
		semanticObjectDelete,
		semanticObjectMove,
		semanticVaultReindex,
		semanticVaultCheck,
		semanticReadSearch,
		semanticReadFile,
		semanticReadBacklinks,
		semanticReadOutlinks,
		semanticReadResolve,
		semanticReadQuery,
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
		semanticSchemaTypeTemplateList,
		semanticSchemaTypeTemplateSet,
		semanticSchemaTypeTemplateRemove,
		semanticSchemaTypeTemplateDefault,
		semanticSchemaCoreTemplateList,
		semanticSchemaCoreTemplateSet,
		semanticSchemaCoreTemplateRemove,
		semanticSchemaCoreTemplateDefault,
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
		semanticWorkflowRunsPrune:
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
	case semanticVaultReindex:
		out, isErr := s.callDirectReindex(args)
		return out, isErr, true
	case semanticVaultCheck:
		out, isErr := s.callDirectCheck(args)
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
	case semanticSchemaTypeTemplateList:
		out, isErr := s.callDirectSchemaTypeTemplateList(args)
		return out, isErr, true
	case semanticSchemaTypeTemplateSet:
		out, isErr := s.callDirectSchemaTypeTemplateSet(args)
		return out, isErr, true
	case semanticSchemaTypeTemplateRemove:
		out, isErr := s.callDirectSchemaTypeTemplateRemove(args)
		return out, isErr, true
	case semanticSchemaTypeTemplateDefault:
		out, isErr := s.callDirectSchemaTypeTemplateDefault(args)
		return out, isErr, true
	case semanticSchemaCoreTemplateList:
		out, isErr := s.callDirectSchemaCoreTemplateList(args)
		return out, isErr, true
	case semanticSchemaCoreTemplateSet:
		out, isErr := s.callDirectSchemaCoreTemplateSet(args)
		return out, isErr, true
	case semanticSchemaCoreTemplateRemove:
		out, isErr := s.callDirectSchemaCoreTemplateRemove(args)
		return out, isErr, true
	case semanticSchemaCoreTemplateDefault:
		out, isErr := s.callDirectSchemaCoreTemplateDefault(args)
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
	op, ok := compatibilityToolSemanticMap[name]
	if !ok {
		return "", false, false
	}

	out, isErr, handled := s.callSemanticTool(op, args)
	if !handled {
		return errorEnvelope(
			"INTERNAL_ERROR",
			"semantic handler is not configured",
			"report this issue with the failing tool name and semantic operation",
			map[string]interface{}{"tool_name": name, "semantic_op": op},
		), true, true
	}
	return out, isErr, true
}
