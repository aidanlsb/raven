# Vault Onboarding Guide

This guide helps agents onboard users to a new Raven vault through an interactive, conversational setup process.

## When to Use This Guide

- User just ran `rvn init` and has a fresh vault
- User asks to "set up Raven" or "get started"
- User runs the `onboard` workflow via `raven_workflow_run(name="onboard")`
- Vault has only default types (person, project) and no custom content

## Onboarding Philosophy

**Start simple, grow organically.** Don't front-load complexity. Let the schema evolve from the user's actual needs, not hypothetical ones.

**Show, don't just configure.** Create real content so the user sees immediate value. A query that returns results is more compelling than an empty schema.

**Explain decisions.** Help users understand *why* something is a type vs. a trait, or why certain fields make sense. This builds their mental model.

## Discovery Sequence

Before starting the interview, understand what already exists:

```
# Check current schema
raven_schema(subcommand="types")
raven_schema(subcommand="traits")

# See if there's any existing content
raven_stats()

# Check for saved queries/workflows
raven_query(list=true)
raven_workflow_list()
```

## Interview Flow

### Phase 1: Understand Intent

Ask open-ended questions to understand their use case:

> "What do you want to use this vault for?"

Listen for domains: work, personal, research, creative projects, etc.

> "What kinds of things do you need to keep track of?"

Listen for nouns that become types: meetings, books, articles, recipes, decisions, etc.

### Phase 2: Identify Schema Needs

Based on their answers, identify:

**Types** - Distinct categories of objects with their own structure
- Each type gets its own directory and fields
- Good candidates: things with unique fields or templates

**Traits** - Cross-cutting annotations that apply to many types
- @due, @priority, @todo work across everything
- Good for temporary states or metadata

**Fields** - Structured data on a specific type
- Belongs to one type
- Good for: required info, relationships, type-specific metadata

### Phase 3: Build the Schema

Create types first, then add fields:

```
# Create a type
raven_schema_add_type(
  name="book",
  default_path="books/",
  name_field="title"
)

# Add fields to it
raven_schema_add_field(
  type_name="book",
  field_name="author",
  type="string"
)

raven_schema_add_field(
  type_name="book",
  field_name="status",
  type="enum",
  values="to-read,reading,finished,abandoned"
)
```

Add custom traits if needed:

```
# Add a trait for ratings
raven_schema_add_trait(
  name="rating",
  type="number"
)
```

### Phase 4: Create Seed Content

Ask what they're currently working on and create 2-3 real objects:

```
# Create a project they mentioned
result = raven_new(type="project", title="Website Redesign")

# Add some content
raven_add(
  text="## Goals\n- Modernize the design\n- Improve mobile experience\n\n## Tasks\n- @due(2026-02-15) Create mockups",
  to=result.data.file
)

# Set relevant fields
raven_set(
  object_id="projects/website-redesign",
  fields={"status": "active"}
)
```

If they already have structured JSON exports, offer import instead of manual re-entry:

```
# Preview import first
raven_import(type="project", file="projects.json", dry_run=true)

# Apply after user confirmation
raven_import(type="project", file="projects.json", confirm=true)
```

### Phase 5: Demonstrate Value

Run a query to show immediate value:

```
# Show them their active projects
raven_query(query_string="object:project .status==active")

# Or tasks due soon
raven_query(query_string="trait:due .value==this-week")
```

Offer to save useful queries:

```
raven_query_add(
  name="active",
  query_string="object:project .status==active",
  description="All active projects"
)
```

## Common Type Patterns

### Knowledge Work
- `meeting` - attendees (ref[]), date, action_items
- `decision` - status, stakeholders, rationale
- `project` - status, lead (ref), deadline
- `person` - name, email, company

### Reading/Research
- `book` - title, author, status, rating
- `article` - source, url, read_date
- `quote` - source (ref), page
- `note` - topics[]

### Personal
- `recipe` - ingredients, prep_time, servings
- `goal` - target_date, status, milestones
- `habit` - frequency, streak

## Common Trait Patterns

Already in default schema:
- `@due(YYYY-MM-DD)` - deadlines
- `@priority(low|medium|high)` - importance
- `@todo` - mark items as todo (boolean)

Common additions:
- `@rating(1-5)` - quality rating (number)
- `@context(home|work|errands)` - GTD contexts (enum)
- `@energy(low|medium|high)` - required energy level (enum)
- `@waiting(person-ref)` - blocked on someone (string)
- `@reviewed` - has been processed (boolean)

## Types vs. Traits Decision Guide

**Use a TYPE when:**
- It's a distinct category with unique fields
- You want to query "all X" frequently
- It needs a template or default structure
- Examples: meeting, book, person, project

**Use a TRAIT when:**
- It applies across multiple types
- It's a temporary state or annotation
- You want to query across type boundaries
- Examples: @due, @priority, @todo, @rating

**Use a FIELD when:**
- It's specific to one type
- It's always present (or should be)
- It contains structured data (refs, enums)
- Examples: person.email, book.author, meeting.attendees

## Error Recovery

### User is overwhelmed
Scale back. Suggest starting with just daily notes and one or two types. They can always add more later.

### User wants everything
Gently push back. "Let's start with the 2-3 most important things and add more as you use the system."

### Type vs. trait confusion
Explain with examples: "A book is a type because every book has the same structure. @rating is a trait because you might rate books, articles, recipes—anything."

### Schema mistakes
Schema changes are easy to make. Use `raven_schema_update_*` or `raven_schema_rename_*` to fix issues. Data is in markdown, so nothing is locked in.

## Completion Checklist

A successful onboarding session should:

- [ ] Create 1-3 custom types based on user needs
- [ ] Add relevant fields to those types
- [ ] Optionally add 1-2 custom traits
- [ ] Create 2-3 seed objects with real content
- [ ] Demonstrate at least one query
- [ ] Offer to save useful queries
- [ ] Explain daily notes if relevant
- [ ] Leave user with a clear "what's next"

## What's Next Suggestions

After onboarding, suggest:

1. **Daily notes**: "Try `rvn daily` to start capturing daily notes"
2. **Quick capture**: "Use `rvn add 'your thought'` to quickly capture ideas"
3. **Query exploration**: "Run `rvn query 'object:project'` to see all projects"
4. **Backlinks**: "Create references like [[people/sarah]] to link objects together"
