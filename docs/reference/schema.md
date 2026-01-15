# `schema.yaml` Reference

`schema.yaml` defines the structure of your vault:
- **Types** — what objects are (e.g., `person`, `project`)
- **Traits** — inline annotations (e.g., `@due`, `@priority`)

It lives at the root of your vault.

## Complete Example

```yaml
version: 2

types:
  person:
    name_field: name
    default_path: people/
    fields:
      name: { type: string, required: true }
      email: { type: string }
      company: { type: ref, target: company }
      tags: { type: string[] }

  project:
    name_field: title
    default_path: projects/
    template: templates/project.md
    fields:
      title: { type: string, required: true }
      status: { type: enum, values: [active, paused, done, archived], default: active }
      priority: { type: number, min: 1, max: 5 }
      due: { type: date }
      owner: { type: ref, target: person }

  meeting:
    default_path: meetings/
    fields:
      time: { type: datetime }
      attendees: { type: ref[], target: person }

traits:
  due:
    type: date
  priority:
    type: enum
    values: [low, medium, high]
    default: medium
  highlight:
    type: boolean
  todo:
    type: enum
    values: [todo, done]
```

---

## Built-in Types

Raven has three built-in types that are always available and cannot be modified or removed:

### `page`

The fallback type for files without an explicit `type:` in frontmatter.

```markdown
---
title: Random Note
---

# Random Note
This file has no type, so it's a "page".
```

**Behavior:**
- Any markdown file without `type:` becomes a `page`
- Has no defined fields (all frontmatter keys are allowed but not validated)
- When directory organization is enabled, pages go to `pages/` root

### `section`

Auto-generated for every markdown heading that doesn't have an explicit type declaration.

```markdown
## Tasks
This heading creates a section with ID "file-id#tasks"
```

**Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `title` | string | The heading text |
| `level` | number (1-6) | The heading level (`#` = 1, `##` = 2, etc.) |

**Behavior:**
- Created automatically when parsing documents
- Object ID is `<file-id>#<slugified-heading>`
- Can be referenced like `[[projects/website#tasks]]`
- Overridden when `::type(...)` follows the heading

### `date`

Used for daily notes.

```markdown
---
type: date
---

# Friday, January 10
```

**Behavior:**
- Created automatically by `rvn daily`
- Has no defined fields (use traits for metadata)
- Date references (`[[2026-01-10]]`) resolve to daily notes
- Files are stored in `daily_directory` (from `raven.yaml`)

---

## Type Definitions

### Type Properties

| Property | Type | Description |
|----------|------|-------------|
| `name_field` | string | Field that serves as the display name |
| `default_path` | string | Directory where new files are created |
| `template` | string | Template file path or inline content |
| `fields` | object | Field definitions for frontmatter |

### `name_field`

Designates which field serves as the display name for objects of this type.

```yaml
types:
  person:
    name_field: name
    fields:
      name: { type: string, required: true }

  book:
    name_field: title
    fields:
      title: { type: string, required: true }
```

**Benefits:**

1. **Auto-population:** The title argument to `rvn new` automatically populates this field
   ```bash
   rvn new person "Freya"  # Sets name: Freya
   ```

2. **Reference resolution:** `[[Display Name]]` can resolve to objects by their name_field value
   ```markdown
   [[The Prose Edda]]  # Can resolve to books/the-prose-edda.md if it has title: The Prose Edda
   ```

**Setting via CLI:**

```bash
rvn schema add type book --name-field title
rvn schema update type person --name-field name
rvn schema update type person --name-field -  # Remove name_field
```

### `default_path`

Directory where `rvn new` creates files of this type.

```yaml
types:
  person:
    default_path: people/
```

With `directories` configured in `raven.yaml`, `default_path` is relative to `objects/`.

### `template`

Template for new files of this type. Can be a file path or inline content.

**File-based:**

```yaml
types:
  meeting:
    template: templates/meeting.md
```

**Inline:**

```yaml
types:
  quick-note:
    template: |
      # {{title}}
      
      Created: {{date}}
```

**Template variables:**

| Variable | Description | Example |
|----------|-------------|---------|
| `{{title}}` | Title passed to `rvn new` | "Team Sync" |
| `{{slug}}` | Slugified title | "team-sync" |
| `{{type}}` | The type name | "meeting" |
| `{{date}}` | Today's date | "2026-01-10" |
| `{{datetime}}` | Current datetime | "2026-01-10T14:30:00Z" |
| `{{year}}` | Current year | "2026" |
| `{{month}}` | Current month (2-digit) | "01" |
| `{{day}}` | Current day (2-digit) | "10" |
| `{{weekday}}` | Day name | "Friday" |
| `{{field.X}}` | Value of field X from `--field` | (user-provided) |

---

## Field Definitions

### Field Properties

| Property | Type | Description | Applies to |
|----------|------|-------------|------------|
| `type` | string | Field type (see below) | All |
| `required` | boolean | Whether field must be present | All |
| `default` | any | Default value | All |
| `values` | string[] | Allowed values | enum, enum[] |
| `target` | string | Referenced type | ref, ref[] |
| `min` | number | Minimum value | number |
| `max` | number | Maximum value | number |

### Field Types

#### `string`

Plain text value.

```yaml
fields:
  name: { type: string }
  name: { type: string, required: true }
  nickname: { type: string, default: "" }
```

#### `number`

Numeric value with optional range constraints.

```yaml
fields:
  priority: { type: number }
  priority: { type: number, min: 1, max: 5 }
  score: { type: number, min: 0, max: 100, default: 0 }
```

#### `date`

Date in `YYYY-MM-DD` format.

```yaml
fields:
  due: { type: date }
  birthday: { type: date }
```

#### `datetime`

Date and time in RFC3339-ish format (e.g., `2026-01-10T09:00`).

```yaml
fields:
  time: { type: datetime }
  created_at: { type: datetime }
```

#### `bool`

Boolean value (`true` or `false`).

```yaml
fields:
  active: { type: bool }
  archived: { type: bool, default: false }
```

#### `enum`

Single value from a predefined list.

```yaml
fields:
  status:
    type: enum
    values: [active, paused, done, archived]
    default: active
```

#### `ref`

Reference to another object by ID.

```yaml
fields:
  owner: { type: ref, target: person }
  company: { type: ref, target: company }
```

In frontmatter, ref values are object IDs:

```yaml
---
type: project
owner: people/freya
company: companies/stark-industries
---
```

#### Array Types

Add `[]` suffix for array fields:

```yaml
fields:
  tags: { type: string[] }
  collaborators: { type: ref[], target: person }
  milestones: { type: date[] }
  priorities: { type: enum[], values: [low, medium, high] }
```

In frontmatter:

```yaml
---
type: project
tags: [web, frontend, urgent]
collaborators:
  - people/freya
  - people/thor
---
```

---

## Trait Definitions

Traits are inline annotations in content, written as `@name` or `@name(value)`.

### Trait Properties

| Property | Type | Description |
|----------|------|-------------|
| `type` | string | Trait type (see below) |
| `values` | string[] | Allowed values (for enum) |
| `default` | string | Default value |

### Trait Types

#### `string`

Free-form text value.

```yaml
traits:
  note:
    type: string
```

Usage: `@note(Remember to follow up)`

#### `date`

Date value with temporal semantics.

```yaml
traits:
  due:
    type: date
```

Usage: `@due(2026-01-15)` or `@due(tomorrow)`

**Special date values for queries:**
- `past` — Before today
- `today` — Today
- `tomorrow` — Tomorrow
- `this-week` — This week
- `next-week` — Next week
- `this-month` — This month

#### `datetime`

Date and time value.

```yaml
traits:
  remind:
    type: datetime
```

Usage: `@remind(2026-01-15T09:00)`

#### `enum`

Single value from a predefined list.

```yaml
traits:
  priority:
    type: enum
    values: [low, medium, high]
    default: medium

  todo:
    type: enum
    values: [todo, done]
```

Usage: `@priority(high)`, `@todo(done)`

#### `bool` / `boolean`

Boolean trait (presence = true).

```yaml
traits:
  highlight:
    type: boolean
```

Usage: `@highlight` (no value needed)

---

## Schema Evolution

What happens when you change the schema.

### Removing a Type

```bash
rvn schema remove type old-type
```

**Effect:**
- Existing files of this type become `page` type (fallback)
- Warns about affected files before removal
- Requires confirmation (use `--force` to skip)

After removal, run `rvn check` to see `unknown_type` issues, then `rvn reindex --full` to update the index.

### Removing a Trait

```bash
rvn schema remove trait old-trait
```

**Effect:**
- Existing `@trait` annotations remain in files
- Annotations are no longer indexed or queryable
- Warns about affected instances before removal

The annotations become inert text until you either:
- Re-add the trait to the schema
- Manually remove the annotations from files

### Adding a Required Field

```bash
rvn schema update field person email --required=true
```

**Effect:**
- Existing objects without this field become invalid
- `rvn check` reports `missing_required_field` issues

**Protection:**
- Raven **blocks** making a field required if any objects lack it
- You must first add the field to all objects, then make it required

```bash
# First, add the field to all objects
rvn query "object:person !.email==*" --ids | rvn set --stdin email="" --confirm

# Then make it required
rvn schema update field person email --required=true
```

### Removing a Required Field

**Protection:**
- Raven blocks removing a field if it's marked required
- First make it optional, then remove it

```bash
rvn schema update field person email --required=false
rvn schema remove field person email
```

### Changing Enum Values

```bash
rvn schema update trait priority --values critical,high,medium,low
```

**Effect:**
- Existing values not in the new list cause `invalid_enum_value` errors
- No automatic migration of existing values

Fix with:

```bash
# Find invalid values
rvn check

# Update them
rvn query "trait:priority value==urgent" --apply "set priority=critical" --confirm
```

### After Schema Changes

Always reindex after schema changes to ensure the index reflects the new schema:

```bash
rvn reindex --full
```

Run `rvn check` to find any validation issues created by the changes.

---

## Validation

### `rvn schema validate`

Checks schema.yaml for internal consistency:
- Valid field types
- Valid trait types
- Enum fields have `values` defined
- Ref fields have valid `target` types
- No circular dependencies

### `rvn check`

Validates vault files against the schema. Reports issues like:

| Issue Type | Description | Fix |
|------------|-------------|-----|
| `unknown_type` | File uses undefined type | Add type to schema |
| `unknown_frontmatter_key` | Field not defined for type | Add field to type |
| `missing_required_field` | Required field not set | Set the field value |
| `invalid_enum_value` | Value not in allowed list | Use valid value |
| `undefined_trait` | Trait not in schema | Add trait to schema |
| `missing_reference` | Link to non-existent page | Create the page |

---

## Reserved Keys

These frontmatter keys are always allowed regardless of type:

| Key | Description |
|-----|-------------|
| `type` | Object type (defaults to `page` if omitted) |
| `id` | Explicit object ID (primarily for embedded objects) |
| `alias` | Alternative name for reference resolution |

### `alias`

The `alias` field enables alternative reference resolution. Any object can have an alias without needing to declare it in the schema:

```yaml
# people/freya.md
---
type: person
name: Freya
alias: The Queen
---
```

Now `[[The Queen]]` resolves to `people/freya`.

Aliases are matched case-insensitively and also in slugified form (e.g., `[[the-queen]]` also works).

---

## CLI Commands

```bash
# View schema
rvn schema types
rvn schema type person
rvn schema traits
rvn schema trait due

# Add to schema
rvn schema add type book --name-field title --default-path books/
rvn schema add trait priority --type enum --values high,medium,low
rvn schema add field person email --type string --required

# Update schema
rvn schema update type person --name-field name
rvn schema update trait priority --values critical,high,medium,low
rvn schema update field person email --required=true

# Rename a type (updates all files)
rvn schema rename type event meeting          # Preview
rvn schema rename type event meeting --confirm # Apply

# Remove from schema
rvn schema remove type old-type
rvn schema remove trait old-trait
rvn schema remove field person nickname

# Validate
rvn schema validate
rvn check
```
