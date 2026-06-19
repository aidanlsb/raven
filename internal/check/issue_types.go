package check

// IssueType categorizes validation issues for programmatic handling.
type IssueType string

const (
	IssueUnknownType             IssueType = "unknown_type"
	IssueMissingReference        IssueType = "missing_reference"
	IssueUndefinedTrait          IssueType = "undefined_trait"
	IssueUnknownFrontmatter      IssueType = "unknown_frontmatter_key"
	IssueDuplicateID             IssueType = "duplicate_object_id"
	IssueMissingRequiredField    IssueType = "missing_required_field"
	IssueInvalidFieldValue       IssueType = "invalid_field_value"
	IssueMissingRequiredTrait    IssueType = "missing_required_trait"
	IssueInvalidEnumValue        IssueType = "invalid_enum_value"
	IssueAmbiguousReference      IssueType = "ambiguous_reference"
	IssueInvalidTraitValue       IssueType = "invalid_trait_value"
	IssueParseError              IssueType = "parse_error"
	IssueWrongTargetType         IssueType = "wrong_target_type"
	IssueInvalidDateFormat       IssueType = "invalid_date_format"
	IssueShortRefCouldBeFullPath IssueType = "short_ref_could_be_full_path"
	IssueStaleIndex              IssueType = "stale_index"
	IssueUnusedType              IssueType = "unused_type"
	IssueUnusedTrait             IssueType = "unused_trait"
	IssueMissingTargetType       IssueType = "missing_target_type"
	IssueSelfReferentialRequired IssueType = "self_referential_required"
	IssueUnknownFieldType        IssueType = "unknown_field_type"
	IssueIDCollision             IssueType = "id_collision"
	IssueDuplicateAlias          IssueType = "duplicate_alias"
	IssueAliasCollision          IssueType = "alias_collision"
	IssueStaleFragment           IssueType = "stale_fragment"
	IssueLocalFragmentRef        IssueType = "local_fragment_ref"
	IssueNonCanonicalPath        IssueType = "non_canonical_path"
	IssueNonCanonicalRef         IssueType = "non_canonical_ref"
	IssueDirectoryTypeMismatch   IssueType = "directory_type_mismatch"
	IssueMissingAsset            IssueType = "missing_asset"
	IssueOrphanedAsset           IssueType = "orphaned_asset"
)

// AllIssueTypes returns the stable issue type strings emitted by check.
func AllIssueTypes() []IssueType {
	return []IssueType{
		IssueUnknownType,
		IssueMissingReference,
		IssueUndefinedTrait,
		IssueUnknownFrontmatter,
		IssueDuplicateID,
		IssueMissingRequiredField,
		IssueInvalidFieldValue,
		IssueMissingRequiredTrait,
		IssueInvalidEnumValue,
		IssueAmbiguousReference,
		IssueInvalidTraitValue,
		IssueParseError,
		IssueWrongTargetType,
		IssueInvalidDateFormat,
		IssueShortRefCouldBeFullPath,
		IssueStaleIndex,
		IssueUnusedType,
		IssueUnusedTrait,
		IssueMissingTargetType,
		IssueSelfReferentialRequired,
		IssueUnknownFieldType,
		IssueIDCollision,
		IssueDuplicateAlias,
		IssueAliasCollision,
		IssueStaleFragment,
		IssueLocalFragmentRef,
		IssueNonCanonicalPath,
		IssueNonCanonicalRef,
		IssueDirectoryTypeMismatch,
		IssueMissingAsset,
		IssueOrphanedAsset,
	}
}

// Issue represents a validation issue.
type Issue struct {
	Level      IssueLevel
	Type       IssueType
	FilePath   string
	Line       int
	Message    string
	Value      string // The problematic value (type name, trait name, ref, etc.)
	FixCommand string // Suggested command to fix the issue
	FixHint    string // Human-readable fix hint
}

// SchemaIssue represents a schema-level validation issue (not file-specific).
type SchemaIssue struct {
	Level      IssueLevel
	Type       IssueType
	Message    string
	Value      string // The type/trait/field name
	FixCommand string
	FixHint    string
}

// IssueLevel indicates the severity of an issue.
type IssueLevel int

const (
	LevelError IssueLevel = iota
	LevelWarning
)

func (l IssueLevel) String() string {
	switch l {
	case LevelError:
		return "ERROR"
	case LevelWarning:
		return "WARN"
	default:
		return "UNKNOWN"
	}
}
