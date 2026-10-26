package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SynthVaultOptions controls a realistic multi-type Markdown vault used by
// hot-path benchmarks. Zero values pick DefaultSynthVaultOptions.
type SynthVaultOptions struct {
	People   int
	Projects int
	Notes    int
	Daily    int
}

// DefaultSynthVaultOptions returns a vault sized to exercise indexing,
// resolution, and check without making `go test -bench` unusable.
func DefaultSynthVaultOptions() SynthVaultOptions {
	return SynthVaultOptions{
		People:   500,
		Projects: 400,
		Notes:    250,
		Daily:    50,
	}
}

// LargeSynthVaultOptions returns a ~2,000-note vault closer to production
// agent workloads.
func LargeSynthVaultOptions() SynthVaultOptions {
	return SynthVaultOptions{
		People:   800,
		Projects: 700,
		Notes:    400,
		Daily:    100,
	}
}

func (o SynthVaultOptions) withDefaults() SynthVaultOptions {
	d := DefaultSynthVaultOptions()
	if o.People == 0 && o.Projects == 0 && o.Notes == 0 && o.Daily == 0 {
		return d
	}
	if o.People == 0 {
		o.People = d.People
	}
	if o.Projects == 0 {
		o.Projects = d.Projects
	}
	if o.Notes == 0 {
		o.Notes = d.Notes
	}
	if o.Daily == 0 {
		o.Daily = d.Daily
	}
	return o
}

// FileCount is the number of Markdown notes the options will write.
func (o SynthVaultOptions) FileCount() int {
	o = o.withDefaults()
	return o.People + o.Projects + o.Notes + o.Daily
}

// SynthSchemaYAML is a schema with types, name fields, ref fields, and traits
// matching everyday Raven vaults.
const SynthSchemaYAML = `version: 2
types:
  person:
    default_path: people/
    name_field: name
    fields:
      name:
        type: string
        required: true
      email:
        type: string
      alias:
        type: string
      team:
        type: ref
        target: project
  project:
    default_path: projects/
    name_field: title
    fields:
      title:
        type: string
        required: true
      status:
        type: enum
        values: [active, paused, completed, abandoned]
        default: active
      owner:
        type: ref
        target: person
      milestones:
        type: date[]
  note:
    default_path: notes/
    name_field: title
    fields:
      title:
        type: string
        required: true
      related:
        type: ref[]
        target: person
traits:
  due:
    type: date
  todo:
    type: boolean
    default: true
  priority:
    type: enum
    values: [low, medium, high]
    default: medium
`

// SynthRavenYAML is a minimal vault config for synthetic vaults.
const SynthRavenYAML = `directories:
  daily: daily/
`

// WriteSynthVault writes schema.yaml, raven.yaml, and Markdown notes under dir.
func WriteSynthVault(dir string, opts SynthVaultOptions) error {
	opts = opts.withDefaults()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "schema.yaml"), []byte(SynthSchemaYAML), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "raven.yaml"), []byte(SynthRavenYAML), 0o644); err != nil {
		return err
	}

	for _, sub := range []string{"people", "projects", "notes", "daily"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return err
		}
	}

	for i := 0; i < opts.People; i++ {
		id := fmt.Sprintf("p%04d", i)
		ownerProj := fmt.Sprintf("projects/proj-%04d", i%max(opts.Projects, 1))
		nextPerson := fmt.Sprintf("people/p%04d", (i+1)%opts.People)
		alias := ""
		if i%17 == 0 {
			alias = fmt.Sprintf("alias: nick-%04d\n", i)
		}
		body := fmt.Sprintf(`---
type: person
name: Person %s
email: %s@example.test
%s---

# Person %s

Works on [[%s]] with [[%s]]. See also [[%s]].

## Notes

- @todo Follow up with [[%s]]
- @due(2026-03-%02d) Send the weekly update
- @priority(medium) Keep the project moving

## Context

This person collaborates across several projects and keeps a running log of
decisions. The body is long enough to exercise Markdown parsing, FTS
indexing, and wikilink extraction without dominating wall-clock time.

Related: [handbook](https://example.test/handbook/%s)
`, id, id, alias, id, ownerProj, nextPerson, sectionLink(opts, i), nextPerson, (i%28)+1, id)
		if err := os.WriteFile(filepath.Join(dir, "people", id+".md"), []byte(body), 0o644); err != nil {
			return err
		}
	}

	statuses := []string{"active", "paused", "completed", "abandoned"}
	for i := 0; i < opts.Projects; i++ {
		id := fmt.Sprintf("proj-%04d", i)
		owner := fmt.Sprintf("people/p%04d", i%max(opts.People, 1))
		peer := fmt.Sprintf("projects/proj-%04d", (i+1)%opts.Projects)
		body := fmt.Sprintf(`---
type: project
title: Project %s
status: %s
owner: "[[%s]]"
milestones:
  - 2026-04-%02d
  - 2026-08-%02d
---

# Project %s

Owned by [[%s]]. Depends on [[%s]].

## Goals

Ship the next milestone and keep [[%s]] unblocked.

## Tasks

- @todo Draft the spec
- @due(2026-05-%02d) Review with [[%s]]
- @priority(%s) Track remaining work

## Details

Project notes cover architecture, rollout, and follow-ups. Wikilinks and
traits here feed the index refs, date_index, and FTS tables.

See [docs](https://example.test/projects/%s).
`, id, statuses[i%len(statuses)], owner, (i%28)+1, (i%28)+1, id, owner, peer, owner, (i%28)+1, owner, []string{"low", "medium", "high"}[i%3], id)
		if err := os.WriteFile(filepath.Join(dir, "projects", id+".md"), []byte(body), 0o644); err != nil {
			return err
		}
	}

	for i := 0; i < opts.Notes; i++ {
		id := fmt.Sprintf("note-%04d", i)
		person := fmt.Sprintf("people/p%04d", i%max(opts.People, 1))
		project := fmt.Sprintf("projects/proj-%04d", i%max(opts.Projects, 1))
		body := fmt.Sprintf(`---
type: note
title: Note %s
related:
  - "[[%s]]"
---

# Note %s

Captured from a meeting about [[%s]] and [[%s]].

## Summary

- Decision recorded for [[%s]]
- @todo File the follow-up
- @due(2026-06-%02d) Check in with [[%s]]

## Transcript

Placeholder paragraphs so parsing and FTS have a non-trivial body to scan.
The note also links to a section: [[%s#goals]].
`, id, person, id, person, project, project, (i%28)+1, person, project)
		if err := os.WriteFile(filepath.Join(dir, "notes", id+".md"), []byte(body), 0o644); err != nil {
			return err
		}
	}

	for i := 0; i < opts.Daily; i++ {
		day := i + 1
		month := (i / 28) + 1
		date := fmt.Sprintf("2026-%02d-%02d", month, (day-1)%28+1)
		person := fmt.Sprintf("people/p%04d", i%max(opts.People, 1))
		body := fmt.Sprintf(`---
type: page
---

# %s

Daily log. Talked with [[%s]] about [[projects/proj-%04d]].

- @todo Inbox zero
- @due(%s) Finish the write-up
`, date, person, i%max(opts.Projects, 1), date)
		if err := os.WriteFile(filepath.Join(dir, "daily", date+".md"), []byte(body), 0o644); err != nil {
			return err
		}
	}

	return nil
}

func sectionLink(opts SynthVaultOptions, i int) string {
	if opts.Projects == 0 {
		return fmt.Sprintf("people/p%04d", i%max(opts.People, 1))
	}
	return fmt.Sprintf("projects/proj-%04d#goals", i%opts.Projects)
}

// BuildSynthVault writes a synthetic vault into a temp directory.
func BuildSynthVault(t testing.TB, opts SynthVaultOptions) string {
	t.Helper()
	dir := t.TempDir()
	if err := WriteSynthVault(dir, opts); err != nil {
		t.Fatalf("write synthetic vault: %v", err)
	}
	return dir
}

// SynthPersonID returns a canonical person object ID from WriteSynthVault.
func SynthPersonID(i int) string {
	return fmt.Sprintf("people/p%04d", i)
}

// WipeIndex removes the derived .raven index so the next open rebuilds it.
func WipeIndex(t testing.TB, vaultPath string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(vaultPath, ".raven")); err != nil {
		t.Fatalf("wipe index: %v", err)
	}
}

// MustReadFile is a tiny helper for mutation benches that need a known note.
func MustReadFile(t testing.TB, vaultPath, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(vaultPath, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// CountMarkdownFiles returns how many .md files WriteSynthVault created.
func CountMarkdownFiles(vaultPath string) (int, error) {
	count := 0
	err := filepath.WalkDir(vaultPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".raven" || name == ".git" || name == ".trash" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".md") {
			count++
		}
		return nil
	})
	return count, err
}
