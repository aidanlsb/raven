// Package codes defines Raven's stable, transport-neutral response codes.
package codes

// ErrorCode is the stable code carried in JSON error envelopes.
type ErrorCode string

// WarningCode is the stable code carried in JSON warning envelopes.
type WarningCode string

const (
	// Vault/config errors.
	ErrVaultNotFound     ErrorCode = "VAULT_NOT_FOUND"
	ErrVaultNotSpecified ErrorCode = "VAULT_NOT_SPECIFIED"
	ErrVaultResolution   ErrorCode = "VAULT_RESOLUTION_FAILED"
	ErrConfigInvalid     ErrorCode = "CONFIG_INVALID"

	// Schema errors.
	ErrSchemaNotFound       ErrorCode = "SCHEMA_NOT_FOUND"
	ErrSchemaInvalid        ErrorCode = "SCHEMA_INVALID"
	ErrSchemaMismatch       ErrorCode = "SCHEMA_MISMATCH"
	ErrTypeNotFound         ErrorCode = "TYPE_NOT_FOUND"
	ErrTraitNotFound        ErrorCode = "TRAIT_NOT_FOUND"
	ErrFieldNotFound        ErrorCode = "FIELD_NOT_FOUND"
	ErrDataIntegrityBlock   ErrorCode = "DATA_INTEGRITY_BLOCK"
	ErrConfirmationRequired ErrorCode = "CONFIRMATION_REQUIRED"

	// Object/reference errors.
	ErrObjectNotFound ErrorCode = "OBJECT_NOT_FOUND"
	ErrObjectExists   ErrorCode = "OBJECT_EXISTS"
	ErrObjectInvalid  ErrorCode = "OBJECT_INVALID"
	ErrRefNotFound    ErrorCode = "REF_NOT_FOUND"
	ErrRefInvalid     ErrorCode = "REF_INVALID"
	ErrRefAmbiguous   ErrorCode = "REF_AMBIGUOUS"

	// File/storage errors.
	ErrFileNotFound     ErrorCode = "FILE_NOT_FOUND"
	ErrFileExists       ErrorCode = "FILE_EXISTS"
	ErrFileRead         ErrorCode = "FILE_READ_ERROR"
	ErrFileWrite        ErrorCode = "FILE_WRITE_ERROR"
	ErrFileOutsideVault ErrorCode = "FILE_OUTSIDE_VAULT"
	ErrDatabase         ErrorCode = "DATABASE_ERROR"
	ErrDatabaseVersion  ErrorCode = "DATABASE_VERSION_MISMATCH"

	// Validation/input errors.
	ErrValidationFailed     ErrorCode = "VALIDATION_FAILED"
	ErrRequiredFieldMissing ErrorCode = "REQUIRED_FIELD_MISSING"
	ErrInvalidValue         ErrorCode = "INVALID_VALUE"
	ErrUnknownField         ErrorCode = "UNKNOWN_FIELD"
	ErrInvalidInput         ErrorCode = "INVALID_INPUT"
	ErrInvalidArgs          ErrorCode = "INVALID_ARGS"
	ErrMissingArgument      ErrorCode = "MISSING_ARGUMENT"
	ErrCommandNotFound      ErrorCode = "COMMAND_NOT_FOUND"
	ErrCommandNotInvokable  ErrorCode = "COMMAND_NOT_INVOKABLE"
	ErrDuplicateName        ErrorCode = "DUPLICATE_NAME"
	ErrPrefixNotFound       ErrorCode = "PREFIX_NOT_FOUND"
	ErrStringNotFound       ErrorCode = "STRING_NOT_FOUND"
	ErrMultipleMatches      ErrorCode = "MULTIPLE_MATCHES"
	ErrNotFound             ErrorCode = "NOT_FOUND"

	// Query errors.
	ErrQueryNotFound ErrorCode = "QUERY_NOT_FOUND"
	ErrQueryInvalid  ErrorCode = "QUERY_INVALID"
	ErrQueryFailed   ErrorCode = "QUERY_FAILED"

	// Skill errors.
	ErrSkillNotFound          ErrorCode = "SKILL_NOT_FOUND"
	ErrSkillNotInstalled      ErrorCode = "SKILL_NOT_INSTALLED"
	ErrSkillTargetUnsupported ErrorCode = "SKILL_TARGET_UNSUPPORTED"
	ErrSkillRenderFailed      ErrorCode = "SKILL_RENDER_FAILED"
	ErrSkillPathUnresolved    ErrorCode = "SKILL_PATH_UNRESOLVED"
	ErrSkillReceiptInvalid    ErrorCode = "SKILL_RECEIPT_INVALID"

	// MCP/tool execution errors.
	ErrMCPClientInvalid   ErrorCode = "MCP_CLIENT_INVALID"
	ErrMCPConfigWrite     ErrorCode = "MCP_CONFIG_WRITE_ERROR"
	ErrExecutableRequired ErrorCode = "EXECUTABLE_REQUIRED"
	ErrUnknownTool        ErrorCode = "UNKNOWN_TOOL"
	ErrExecutionFailed    ErrorCode = "EXECUTION_FAILED"
	ErrExecutionError     ErrorCode = "EXECUTION_ERROR"
	ErrInvalidJSON        ErrorCode = "INVALID_JSON"
	ErrToolReturnedError  ErrorCode = "TOOL_RETURNED_ERROR"
	ErrFetchFailed        ErrorCode = "FETCH_FAILED"
	ErrCancelled          ErrorCode = "CANCELLED"

	// General errors.
	ErrInternal       ErrorCode = "INTERNAL_ERROR"
	ErrNotImplemented ErrorCode = "NOT_IMPLEMENTED"
)

const (
	WarnRefNotFound       WarningCode = "REF_NOT_FOUND"
	WarnDeprecated        WarningCode = "DEPRECATED"
	WarnSchemaOutdated    WarningCode = "SCHEMA_OUTDATED"
	WarnDatabaseOutdated  WarningCode = "DATABASE_OUTDATED"
	WarnIndexUpdateFailed WarningCode = "INDEX_UPDATE_FAILED"
	WarnDocsFetchFailed   WarningCode = "DOCS_FETCH_FAILED"
	WarnWrongCommand      WarningCode = "WRONG_COMMAND"
	WarnMissingField      WarningCode = "MISSING_REQUIRED_FIELD"
	WarnBacklinks         WarningCode = "HAS_BACKLINKS"
	WarnSectionSkipped    WarningCode = "SECTION_SKIPPED"
	WarnUnknownField      WarningCode = "UNKNOWN_FIELD"
	WarnTypeMismatch      WarningCode = "TYPE_DIRECTORY_MISMATCH"
	WarnOrphanedFiles     WarningCode = "ORPHANED_FILES"
	WarnOrphanedTraits    WarningCode = "ORPHANED_TRAITS"
	WarnCheckIncomplete   WarningCode = "CHECK_APPLY_INCOMPLETE"
)

var knownErrorCodes = map[ErrorCode]struct{}{
	ErrVaultNotFound: {}, ErrVaultNotSpecified: {}, ErrVaultResolution: {}, ErrConfigInvalid: {},
	ErrSchemaNotFound: {}, ErrSchemaInvalid: {}, ErrSchemaMismatch: {}, ErrTypeNotFound: {}, ErrTraitNotFound: {}, ErrFieldNotFound: {}, ErrDataIntegrityBlock: {}, ErrConfirmationRequired: {},
	ErrObjectNotFound: {}, ErrObjectExists: {}, ErrObjectInvalid: {}, ErrRefNotFound: {}, ErrRefInvalid: {}, ErrRefAmbiguous: {},
	ErrFileNotFound: {}, ErrFileExists: {}, ErrFileRead: {}, ErrFileWrite: {}, ErrFileOutsideVault: {}, ErrDatabase: {}, ErrDatabaseVersion: {},
	ErrValidationFailed: {}, ErrRequiredFieldMissing: {}, ErrInvalidValue: {}, ErrUnknownField: {}, ErrInvalidInput: {}, ErrInvalidArgs: {}, ErrMissingArgument: {}, ErrCommandNotFound: {}, ErrCommandNotInvokable: {}, ErrDuplicateName: {}, ErrPrefixNotFound: {}, ErrStringNotFound: {}, ErrMultipleMatches: {}, ErrNotFound: {},
	ErrQueryNotFound: {}, ErrQueryInvalid: {}, ErrQueryFailed: {},
	ErrSkillNotFound: {}, ErrSkillNotInstalled: {}, ErrSkillTargetUnsupported: {}, ErrSkillRenderFailed: {}, ErrSkillPathUnresolved: {}, ErrSkillReceiptInvalid: {},
	ErrMCPClientInvalid: {}, ErrMCPConfigWrite: {}, ErrExecutableRequired: {}, ErrUnknownTool: {}, ErrExecutionFailed: {}, ErrExecutionError: {}, ErrInvalidJSON: {}, ErrToolReturnedError: {}, ErrFetchFailed: {}, ErrCancelled: {},
	ErrInternal: {}, ErrNotImplemented: {},
}

var knownWarningCodes = map[WarningCode]struct{}{
	WarnRefNotFound: {}, WarnDeprecated: {}, WarnSchemaOutdated: {}, WarnDatabaseOutdated: {}, WarnIndexUpdateFailed: {}, WarnDocsFetchFailed: {},
	WarnWrongCommand: {}, WarnMissingField: {}, WarnBacklinks: {}, WarnSectionSkipped: {}, WarnUnknownField: {}, WarnTypeMismatch: {},
	WarnOrphanedFiles: {}, WarnOrphanedTraits: {}, WarnCheckIncomplete: {},
}

// IsErrorCode reports whether code is part of Raven's stable error contract.
func IsErrorCode(code string) bool {
	_, ok := knownErrorCodes[ErrorCode(code)]
	return ok
}

// IsWarningCode reports whether code is part of Raven's stable warning contract.
func IsWarningCode(code string) bool {
	_, ok := knownWarningCodes[WarningCode(code)]
	return ok
}
