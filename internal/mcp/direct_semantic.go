package mcp

type semanticOp string

type semanticToolHandler func(*Server, map[string]interface{}) (string, bool)

const (
	semanticObjectCreate              semanticOp = "object.create"
	semanticObjectUpsert              semanticOp = "object.upsert"
	semanticObjectAppend              semanticOp = "object.append"
	semanticObjectSetFields           semanticOp = "object.set_fields"
	semanticObjectDelete              semanticOp = "object.delete"
	semanticObjectMove                semanticOp = "object.move"
	semanticReadSearch                semanticOp = "read.search"
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
)

var semanticToolHandlers = map[semanticOp]semanticToolHandler{
	semanticObjectCreate:              (*Server).callDirectNew,
	semanticObjectUpsert:              (*Server).callDirectUpsert,
	semanticObjectAppend:              (*Server).callDirectAdd,
	semanticObjectSetFields:           (*Server).callDirectSet,
	semanticObjectDelete:              (*Server).callDirectDelete,
	semanticObjectMove:                (*Server).callDirectMove,
	semanticReadSearch:                (*Server).callDirectSearch,
	semanticReadBacklinks:             (*Server).callDirectBacklinks,
	semanticReadOutlinks:              (*Server).callDirectOutlinks,
	semanticReadResolve:               (*Server).callDirectResolve,
	semanticReadQuery:                 (*Server).callDirectQuery,
	semanticSchemaAddType:             (*Server).callDirectSchemaAddType,
	semanticSchemaAddTrait:            (*Server).callDirectSchemaAddTrait,
	semanticSchemaAddField:            (*Server).callDirectSchemaAddField,
	semanticSchemaValidate:            (*Server).callDirectSchemaValidate,
	semanticSchemaUpdateType:          (*Server).callDirectSchemaUpdateType,
	semanticSchemaUpdateTrait:         (*Server).callDirectSchemaUpdateTrait,
	semanticSchemaUpdateField:         (*Server).callDirectSchemaUpdateField,
	semanticSchemaRemoveType:          (*Server).callDirectSchemaRemoveType,
	semanticSchemaRemoveTrait:         (*Server).callDirectSchemaRemoveTrait,
	semanticSchemaRemoveField:         (*Server).callDirectSchemaRemoveField,
	semanticSchemaRenameField:         (*Server).callDirectSchemaRenameField,
	semanticSchemaRenameType:          (*Server).callDirectSchemaRenameType,
	semanticSchemaTemplateList:        (*Server).callDirectSchemaTemplateList,
	semanticSchemaTemplateGet:         (*Server).callDirectSchemaTemplateGet,
	semanticSchemaTemplateSet:         (*Server).callDirectSchemaTemplateSet,
	semanticSchemaTemplateRemove:      (*Server).callDirectSchemaTemplateRemove,
	semanticSchemaTypeTemplateList:    (*Server).callDirectSchemaTypeTemplateList,
	semanticSchemaTypeTemplateSet:     (*Server).callDirectSchemaTypeTemplateSet,
	semanticSchemaTypeTemplateRemove:  (*Server).callDirectSchemaTypeTemplateRemove,
	semanticSchemaTypeTemplateDefault: (*Server).callDirectSchemaTypeTemplateDefault,
	semanticSchemaCoreTemplateList:    (*Server).callDirectSchemaCoreTemplateList,
	semanticSchemaCoreTemplateSet:     (*Server).callDirectSchemaCoreTemplateSet,
	semanticSchemaCoreTemplateRemove:  (*Server).callDirectSchemaCoreTemplateRemove,
	semanticSchemaCoreTemplateDefault: (*Server).callDirectSchemaCoreTemplateDefault,
}

var compatibilityToolSemanticMap = map[string]semanticOp{
	"raven_new":                          semanticObjectCreate,
	"raven_upsert":                       semanticObjectUpsert,
	"raven_add":                          semanticObjectAppend,
	"raven_set":                          semanticObjectSetFields,
	"raven_delete":                       semanticObjectDelete,
	"raven_move":                         semanticObjectMove,
	"raven_search":                       semanticReadSearch,
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
}

func (s *Server) callToolDirect(name string, args map[string]interface{}) (string, bool, bool) {
	op, ok := compatibilityToolSemanticMap[name]
	if !ok {
		return "", false, false
	}

	handler, ok := semanticToolHandlers[op]
	if !ok {
		return errorEnvelope(
			"INTERNAL_ERROR",
			"semantic handler is not configured",
			"report this issue with the failing tool name and semantic operation",
			map[string]interface{}{"tool_name": name, "semantic_op": op},
		), true, true
	}

	out, isErr := handler(s, args)
	return out, isErr, true
}
