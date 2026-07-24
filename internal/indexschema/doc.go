// Package indexschema defines the shared contract between the SQLite index and
// packages that generate SQL against it.
//
// SchemaSQL is the authoritative DDL. SQL-generating consumers may rely on the
// table and column shapes it declares. The package also owns the SQL and
// resolver builders shared by index and query so those consumers do not depend
// on index implementation details.
package indexschema
