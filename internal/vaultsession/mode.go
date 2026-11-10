package vaultsession

// Mode selects which vault dependencies a session loads. Named constructors
// replace the old Options soup so callers name the operation class instead of
// combining SkipConfig/SkipSchema/RequireSchema/OpenDB flags.
type Mode struct {
	kind    modeKind
	openDB  bool
	refresh RefreshPolicy
}

type modeKind int

const (
	modeRequired modeKind = iota
	modeConfig
	modeConfigOnly
	modeSchemaMutation
	modeSchemaFirst
	modeLazy
	modeRead
)

// RefreshPolicy is the command-level refresh override for index-backed reads.
// NoRefresh always skips. A non-nil Refresh pointer overrides vault
// auto_reindex. Both unset follows raven.yaml (default on).
type RefreshPolicy struct {
	NoRefresh bool
	Refresh   *bool
}

// Required loads config and a required schema. The index is opened on demand.
func Required() Mode {
	return Mode{kind: modeRequired}
}

// RequiredDB is Required plus an eager index open.
func RequiredDB() Mode {
	return Mode{kind: modeRequired, openDB: true}
}

// Config loads config and schema; a schema load failure is tolerated and
// recorded on the runtime for the caller to surface.
func Config() Mode {
	return Mode{kind: modeConfig}
}

// ConfigOnly loads raven.yaml and skips schema.yaml.
func ConfigOnly() Mode {
	return Mode{kind: modeConfigOnly}
}

// SchemaMutation loads a required schema, opens the index, and treats an
// invalid raven.yaml as non-fatal so schema writes can still proceed.
func SchemaMutation() Mode {
	return Mode{kind: modeSchemaMutation}
}

// SchemaFirst loads schema before config and requires a valid schema.
func SchemaFirst() Mode {
	return Mode{kind: modeSchemaFirst}
}

// Lazy skips config and schema. Callers that need raven.yaml load it later.
func Lazy() Mode {
	return Mode{kind: modeLazy}
}

// Read loads config and a tolerated schema, then refreshes and opens the index
// according to policy.
func Read(policy RefreshPolicy) Mode {
	return Mode{kind: modeRead, refresh: policy, openDB: true}
}
