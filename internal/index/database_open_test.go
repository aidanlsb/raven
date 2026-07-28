package index

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/filelock"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestLinksTableSchema(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	rows, err := db.db.Query(`PRAGMA table_info(links)`)
	if err != nil {
		t.Fatalf("query links schema: %v", err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var (
			cid       int
			name      string
			colType   string
			notNull   int
			defaultV  sql.NullString
			primaryID int
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultV, &primaryID); err != nil {
			t.Fatalf("scan links schema: %v", err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate links schema: %v", err)
	}

	want := []string{
		"id", "source_id", "source_type", "file_path", "line_number",
		"position_start", "position_end", "raw_target", "display", "is_image",
		"scheme", "ext", "normalized_key",
	}
	if len(columns) != len(want) {
		t.Fatalf("links columns = %#v, want %#v", columns, want)
	}
	for i := range want {
		if columns[i] != want[i] {
			t.Fatalf("links columns = %#v, want %#v", columns, want)
		}
	}
}

func TestDatabaseConfiguresSQLiteConcurrency(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer db.Close()

	if got := db.DB().Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1", got)
	}
	var busyTimeout int
	if err := db.DB().QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("read busy_timeout: %v", err)
	}
	if busyTimeout != sqliteBusyTimeoutMillis {
		t.Fatalf("busy_timeout = %d, want %d", busyTimeout, sqliteBusyTimeoutMillis)
	}
}
func TestOpenWithRebuildLock(t *testing.T) {
	t.Parallel()
	vaultDir := t.TempDir()
	dbDir := filepath.Join(vaultDir, ".raven")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		t.Fatalf("failed to create db dir: %v", err)
	}

	lockPath := filepath.Join(dbDir, "index.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("failed to open lock file: %v", err)
	}
	defer lockFile.Close()

	if err := filelock.TryLockExclusive(lockFile); err != nil {
		t.Fatalf("failed to acquire test lock: %v", err)
	}
	defer filelock.Unlock(lockFile)

	if _, err := OpenWithRebuild(vaultDir, RebuildOptions{}); !errors.Is(err, ErrIndexLocked) {
		t.Fatalf("expected ErrIndexLocked, got %v", err)
	}
}
func TestOpenWithRebuildBlocksOrdinaryOpenForSessionLifetime(t *testing.T) {
	t.Parallel()

	vaultDir := t.TempDir()
	db, err := Open(vaultDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close initial database: %v", err)
	}

	session, err := OpenWithRebuild(vaultDir, RebuildOptions{})
	if err != nil {
		t.Fatalf("OpenWithRebuild: %v", err)
	}
	if _, err := Open(vaultDir); !errors.Is(err, ErrIndexLocked) {
		t.Fatalf("Open during rebuild error = %v, want ErrIndexLocked", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close rebuild session: %v", err)
	}

	reopened, err := Open(vaultDir)
	if err != nil {
		t.Fatalf("Open after rebuild session closed: %v", err)
	}
	defer reopened.Close()
}
func TestIsSchemaCompatibleUsesMetaVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version *string
		want    bool
	}{
		{
			name: "current version is compatible",
			version: func() *string {
				v := strconv.Itoa(CurrentDBVersion)
				return &v
			}(),
			want: true,
		},
		{
			name: "stale version is incompatible",
			version: func() *string {
				v := strconv.Itoa(CurrentDBVersion - 1)
				return &v
			}(),
			want: false,
		},
		{
			name: "missing version is incompatible",
			want: false,
		},
		{
			name: "invalid version is incompatible",
			version: func() *string {
				v := "banana"
				return &v
			}(),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rawDB, err := sql.Open("sqlite", ":memory:")
			if err != nil {
				t.Fatalf("open db: %v", err)
			}
			defer rawDB.Close()

			if _, err := rawDB.Exec(`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
				t.Fatalf("create meta table: %v", err)
			}
			if tt.version != nil {
				if _, err := rawDB.Exec(`INSERT INTO meta (key, value) VALUES ('version', ?)`, *tt.version); err != nil {
					t.Fatalf("insert version: %v", err)
				}
			}

			if got := isSchemaCompatible(rawDB); got != tt.want {
				t.Fatalf("isSchemaCompatible() = %v, want %v", got, tt.want)
			}
		})
	}
}
func TestOpenRejectsIncompatibleSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
	}{
		{name: "stale version", version: strconv.Itoa(CurrentDBVersion - 1)},
		{name: "missing version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vaultDir := t.TempDir()
			seedLegacyIndexVersion(t, vaultDir, tt.version)

			if _, err := Open(vaultDir); !errors.Is(err, ErrIndexRebuildRequired) {
				t.Fatalf("Open error = %v, want ErrIndexRebuildRequired", err)
			}
		})
	}
}
func TestRebuildSessionKeepsWipedIndexUnavailableUntilComplete(t *testing.T) {
	t.Parallel()

	vaultDir := t.TempDir()
	seedLegacyIndexVersion(t, vaultDir, strconv.Itoa(CurrentDBVersion-1))

	session, err := OpenWithRebuild(vaultDir, RebuildOptions{})
	if err != nil {
		t.Fatalf("OpenWithRebuild: %v", err)
	}
	if !session.SchemaRebuilt() {
		t.Fatal("expected stale version to rebuild the schema")
	}

	currentVersion, ok, err := storedDatabaseVersion(session.Database().db)
	if err != nil {
		t.Fatalf("storedDatabaseVersion after rebuild: %v", err)
	}
	if !ok || currentVersion != CurrentDBVersion {
		t.Fatalf("expected rebuilt DB version %d, got ok=%v version=%d", CurrentDBVersion, ok, currentVersion)
	}

	if _, err := Open(vaultDir); !errors.Is(err, ErrIndexLocked) {
		t.Fatalf("Open during rebuild error = %v, want ErrIndexLocked", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close incomplete rebuild session: %v", err)
	}
	if _, err := Open(vaultDir); !errors.Is(err, ErrIndexRebuildRequired) {
		t.Fatalf("Open after interrupted rebuild error = %v, want ErrIndexRebuildRequired", err)
	}

	retry, err := OpenWithRebuild(vaultDir, RebuildOptions{})
	if err != nil {
		t.Fatalf("retry OpenWithRebuild: %v", err)
	}
	defer retry.Close()
	if !retry.SchemaRebuilt() {
		t.Fatal("expected interrupted rebuild to require another schema rebuild")
	}
	doc := &parser.ParsedDocument{
		FilePath: "rebuilt.md",
		Objects: []*model.Object{{
			ID:        "rebuilt",
			Type:      "page",
			Fields:    map[string]fieldvalue.FieldValue{},
			LineStart: 1,
		}},
	}
	if err := retry.Database().IndexDocument(doc, schema.New()); err != nil {
		t.Fatalf("index rebuilt document: %v", err)
	}
	if err := retry.Complete(); err != nil {
		t.Fatalf("complete retry: %v", err)
	}
	if err := retry.Close(); err != nil {
		t.Fatalf("close completed rebuild session: %v", err)
	}

	db, err := Open(vaultDir)
	if err != nil {
		t.Fatalf("Open after completed rebuild: %v", err)
	}
	defer db.Close()
	stats, err := db.Stats()
	if err != nil {
		t.Fatalf("Stats after completed rebuild: %v", err)
	}
	if stats.ObjectCount != 1 {
		t.Fatalf("object count after completed rebuild = %d, want 1", stats.ObjectCount)
	}
}
func seedLegacyIndexVersion(t *testing.T, vaultDir string, version string) {
	t.Helper()

	dbDir := filepath.Join(vaultDir, ".raven")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		t.Fatalf("create db dir: %v", err)
	}

	rawDB, err := sql.Open("sqlite", filepath.Join(dbDir, "index.db"))
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer rawDB.Close()

	if _, err := rawDB.Exec(`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatalf("create meta table: %v", err)
	}
	if version != "" {
		if _, err := rawDB.Exec(`INSERT INTO meta (key, value) VALUES ('version', ?)`, version); err != nil {
			t.Fatalf("insert version: %v", err)
		}
	}
}
