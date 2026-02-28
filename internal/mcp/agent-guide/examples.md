# Example Conversations

**User**: "Find open todos from my experiment meetings"
```
→ Compose query: trait:todo .value==todo within(object:meeting) refs([[projects/experiments]])
→ If unclear which project, also try: trait:todo .value==todo within(object:meeting) content("experiment")
→ Consolidate and present results
```

**User**: "What do I have due this week?"
```
→ raven_query(query_string="trait:due .value==this-week")
→ Summarize results for user
```

**User**: "Show me due items that are past OR today"
```
→ Use membership rather than array quantifiers:
  raven_query(query_string="trait:due in(.value, [past,today])")
```

**User**: "Show me highlights from the books I'm reading"
```
→ raven_query(query_string="trait:highlight on(object:book .status==reading)")
→ If no results, check: raven_schema(subcommand="type", name="book") to verify status field exists
```

**User**: "Tasks related to the website project"
```
→ Try multiple interpretations:
  - trait:todo refs([[projects/website]]) (todos that reference it)
  - trait:todo within([[projects/website]]) (todos inside it)
→ Consolidate results from both
```

**User**: "Add a new person for my colleague Thor Odinson"
```
→ raven_schema(subcommand="type", name="person")  # Check required fields and name_field
→ If name_field: name is set:
   raven_new(type="person", title="Thor Odinson")  # name auto-populated
→ If no name_field:
   raven_new(type="person", title="Thor Odinson", field={"name": "Thor Odinson"})
```

**User**: "My vault has a lot of broken links, can you help fix them?"
```
→ raven_check(issues="missing_reference")  # Focus on broken links
→ Review summary, explain to user
→ "I see 2798 missing references. The most-referenced missing pages are:
   - 'bifrost-bridge' (referenced 15 times)
   - 'Baldur' (referenced 12 times)
  Would you like me to create pages for the most common ones? What type should they be?"
→ Create pages based on user input
```

**User**: "I just created some new projects, make sure they're set up correctly"
```
→ raven_check(type="project")  # Validate all project objects
→ Report any issues: "All 5 projects are valid" or "2 projects have issues: ..."
→ Offer to fix any problems found
```

**User**: "Check if my due dates are formatted correctly"
```
→ raven_check(trait="due")  # Validate all @due trait usages
→ Report: "Found 3 invalid date formats: ..." or "All 42 due dates are valid"
```

**User**: "Create a project for the website redesign"
```
→ raven_schema(subcommand="type", name="project")  # Check fields/traits
→ raven_new(type="project", title="Website Redesign")
→ "Created projects/website-redesign.md. Would you like to set any fields like client or due date?"
```

**User**: "I exported my contacts as JSON. Can you import them?"
```
→ "Absolutely — I'll preview the import first so we can confirm changes."
→ raven_import(type="person", file="contacts.json", dry_run=true)
→ "Preview shows 42 creates, 3 updates, 1 skipped (missing match key). Want me to apply it?"
→ Wait for explicit user approval
→ raven_import(type="person", file="contacts.json", confirm=true)
→ "Done. Imported contacts into your person objects."
```

**User**: "I want a template for my meeting notes"
```
→ Ask: "What sections would you like in your meeting template? Common ones include 
  Attendees, Agenda, Notes, and Action Items."
→ Confirm the type exists and inspect its fields:
  raven_schema(subcommand="type", name="meeting")
→ Create/update the template file:
  raven_template_write(path="meeting.md", content="# {{title}}\n\n## Attendees\n\n## Agenda\n\n## Notes\n\n## Action Items")
→ Register a schema template definition:
  raven_schema_template_set(template_id="meeting_standard", file="templates/meeting.md")
→ Bind it to the meeting type and set as default:
  raven_schema_type_template_set(type_name="meeting", template_id="meeting_standard")
  raven_schema_type_template_default(type_name="meeting", template_id="meeting_standard")
→ Optional smoke test:
  raven_new(type="meeting", title="Team Sync")
→ "Done! New meeting notes will start with that structure."
```

**User**: "What happened yesterday?"
```
→ raven_date(date="yesterday")
→ Summarize: daily note content, items that were due, any meetings
```

**User**: "Open the cursor company page"
```
→ raven_open(reference="cursor")
→ "Opening companies/cursor.md"
```

**User**: "Delete the old bifrost project"
```
→ raven_backlinks(target="projects/old-bifrost")  # ALWAYS check for references first
→ "Before I delete projects/old-bifrost, I want to let you know it's referenced by 
  5 other pages. Deleting it will create broken links. 
  Should I proceed, or would you like to update those references first?"
→ Wait for explicit user confirmation
→ Only after user says yes: raven_delete(object_id="projects/old-bifrost")
→ "Done. The file has been moved to .trash/ and can be recovered if needed."
```

**User**: "Run the meeting prep workflow for my 1:1 with Freya"
```
→ raven_workflow_list()  # Check if meeting-prep exists
→ raven_workflow_run(name="meeting-prep", input={"person_id": "people/freya"})
→ Use the returned prompt (and fetch step output via raven_workflow_runs_step if needed) to provide a comprehensive meeting prep
```

**User**: "I want to save a query for finding all my reading list items"
```
→ raven_query_add(name="reading-list", 
                 query_string="trait:toread", 
                 description="Books and articles to read")
→ "Created saved query 'reading-list'. You can now run it with 'rvn query reading-list'"
```

**User**: "Show me pages that need to be organized"
```
→ raven_untyped()
→ "I found 15 pages without explicit types. Here are the most recent:
  - inbox/random-note.md
  - ideas/app-concept.md
  Would you like to assign types to any of these?"
```

**User**: "Meetings where we discussed the API"
```
→ Try: object:meeting content("API")
→ Or: object:meeting refs([[projects/api]]) if there's an API project
```

**User**: "Overdue items assigned to Freya"
```
→ trait:due .value==past refs([[people/freya]])
```

**User**: "Show my todos sorted by due date"
```
→ Sorting/grouping is not supported in the query language.
→ Run a query (e.g. trait:todo .value==todo) and sort client-side if needed.
```

**User**: "Which projects are mentioned in meetings?"
```
→ object:project refd(object:meeting)
→ This uses the refd: predicate to find projects referenced by meetings
```

**User**: "Find high-priority items that are also due soon"
```
→ trait:due at(trait:priority .value==high)
→ Uses at: to find traits co-located on the same line
```

**User**: "Group my todos by project"
```
→ Grouping is not supported in the query language.
→ Use multiple queries or client-side aggregation after fetching results.
```

**User**: "Sort projects by their earliest due date"
```
→ Sorting is not supported in the query language.
→ Fetch projects and due traits separately, then compute/sort client-side.
```
