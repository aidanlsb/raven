# Command Map

## Creating content

- Create a brand-new typed object: `rvn new <type> <title> --json`
- Append notes or capture text: `rvn add <text> --json` or `rvn add <text> --to <path> --json`
- Write idempotent canonical output: `rvn upsert <type> <title> --json`

## Reading content

- Inspect exact file text before editing: `rvn read <path-or-ref> --raw --json`
- Resolve a short reference to full object ID: `rvn resolve <reference> --json`

## Updating content

- Update frontmatter fields: `rvn set <object_id> key=value --json`
- Surgical body replacement: `rvn edit <path> <old> <new> --json`
- Update a trait value by trait ID: `rvn update <trait_id> <new_value> --json`

## Organizing content

- Change an object's type safely: `rvn reclassify <object> <new-type> --json`
- Safe move or rename with ref updates: `rvn move <source> <dest> --json`
- Safe delete with backlink warnings: `rvn delete <object_id> --json`

## Daily notes

- Open or create today's daily note: `rvn daily --json`
- Open a specific date's note: `rvn daily <date> --json`
- View all activity for a date: `rvn date <date> --json`
- Open a file in configured editor: `rvn open <reference> --json`
