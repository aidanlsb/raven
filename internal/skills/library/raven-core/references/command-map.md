# Command Map

- Create typed object: `rvn new <type> <title> --json`
- Idempotent state write: `rvn upsert <type> <title> --json`
- Read file/object: `rvn read <path-or-ref> --json`
- Query objects/traits: `rvn query <rql-or-saved> --json`
- Search text: `rvn search <query> --json`
- Update frontmatter fields: `rvn set <object_id> key=value --json`
- Surgical replacement: `rvn edit <path> <old> <new> --json`
- Safe move/rename: `rvn move <source> <dest> --json`
- Safe delete: `rvn delete <object_id> --json`
