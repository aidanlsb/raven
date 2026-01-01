//! CLI command implementations

use anyhow::{Context, Result};
use std::path::Path;
use walkdir::WalkDir;

use crate::index::Database;
use crate::parser::parse_document;
use crate::schema::load_schema;

/// Initialize a new vault
pub fn init(path: &Path) -> Result<()> {
    println!("Initializing vault at: {}", path.display());
    
    // Create directory if it doesn't exist
    std::fs::create_dir_all(path)
        .with_context(|| format!("Failed to create vault directory: {}", path.display()))?;
    
    // Create .raven directory
    let raven_dir = path.join(".raven");
    std::fs::create_dir_all(&raven_dir)?;
    
    // Create default schema.yaml
    crate::schema::create_default_schema(path)?;
    
    println!("✓ Created schema.yaml");
    println!("✓ Created .raven/ directory");
    println!("\nVault initialized! Start adding markdown files.");
    
    Ok(())
}

/// Validate the vault (check for errors)
pub fn check(vault_path: &Path, strict: bool) -> Result<()> {
    println!("Checking vault: {}", vault_path.display());
    
    let schema = load_schema(vault_path)?;
    let mut errors = 0;
    let mut warnings = 0;
    let mut file_count = 0;
    
    // Walk all markdown files
    for entry in WalkDir::new(vault_path)
        .into_iter()
        .filter_map(|e| e.ok())
        .filter(|e| {
            e.path().extension().map_or(false, |ext| ext == "md")
                && !e.path().to_string_lossy().contains(".raven")
        })
    {
        file_count += 1;
        let file_path = entry.path();
        let relative_path = file_path.strip_prefix(vault_path).unwrap_or(file_path);
        
        // Read and parse the file
        let content = match std::fs::read_to_string(file_path) {
            Ok(c) => c,
            Err(e) => {
                println!("ERROR: {} - Failed to read: {}", relative_path.display(), e);
                errors += 1;
                continue;
            }
        };
        
        let doc = match parse_document(&content, file_path, vault_path) {
            Ok(d) => d,
            Err(e) => {
                println!("ERROR: {} - Parse error: {}", relative_path.display(), e);
                errors += 1;
                continue;
            }
        };
        
        // Validate each object
        for obj in &doc.objects {
            // Check if type is defined
            if !schema.types.contains_key(&obj.object_type) && obj.object_type != "page" {
                println!("ERROR: {}:{} - Unknown type '{}'", 
                    relative_path.display(), obj.line_start, obj.object_type);
                errors += 1;
            }
            
            // Check embedded objects have IDs
            if obj.heading.is_some() && !obj.id.contains('#') {
                println!("ERROR: {}:{} - Embedded object missing 'id' field",
                    relative_path.display(), obj.line_start);
                errors += 1;
            }
            
            // Validate fields against schema
            if let Some(type_def) = schema.types.get(&obj.object_type) {
                let field_errors = crate::schema::validate_fields(&obj.fields, &type_def.fields, &schema);
                for err in field_errors {
                    println!("ERROR: {}:{} - {}", relative_path.display(), obj.line_start, err);
                    errors += 1;
                }
            }
        }
        
        // Check for undefined traits
        for trait_item in &doc.traits {
            if !schema.traits.contains_key(&trait_item.trait_type) {
                println!("WARN:  {}:{} - Undefined trait '@{}' will be skipped",
                    relative_path.display(), trait_item.line, trait_item.trait_type);
                warnings += 1;
            }
        }
    }
    
    println!();
    if errors == 0 && warnings == 0 {
        println!("✓ No issues found in {} files.", file_count);
    } else {
        println!("Found {} error(s), {} warning(s) in {} files.", errors, warnings, file_count);
    }
    
    if errors > 0 || (strict && warnings > 0) {
        std::process::exit(1);
    }
    
    Ok(())
}

/// Reindex all files
pub fn reindex(vault_path: &Path) -> Result<()> {
    println!("Reindexing vault: {}", vault_path.display());
    
    let _schema = load_schema(vault_path)?;
    let mut db = Database::open(vault_path)?;
    
    let mut file_count = 0;
    let mut error_count = 0;
    
    // Walk all markdown files
    for entry in WalkDir::new(vault_path)
        .into_iter()
        .filter_map(|e| e.ok())
        .filter(|e| {
            e.path().extension().map_or(false, |ext| ext == "md")
                && !e.path().to_string_lossy().contains(".raven")
        })
    {
        let file_path = entry.path();
        let relative_path = file_path.strip_prefix(vault_path).unwrap_or(file_path);
        
        // Read and parse
        let content = match std::fs::read_to_string(file_path) {
            Ok(c) => c,
            Err(e) => {
                eprintln!("Error reading {}: {}", relative_path.display(), e);
                error_count += 1;
                continue;
            }
        };
        
        let doc = match parse_document(&content, file_path, vault_path) {
            Ok(d) => d,
            Err(e) => {
                eprintln!("Error parsing {}: {}", relative_path.display(), e);
                error_count += 1;
                continue;
            }
        };
        
        // Index the document
        if let Err(e) = db.index_document(&doc) {
            eprintln!("Error indexing {}: {}", relative_path.display(), e);
            error_count += 1;
            continue;
        }
        
        file_count += 1;
    }
    
    let stats = db.stats()?;
    
    println!();
    println!("✓ Indexed {} files", file_count);
    println!("  {} objects", stats.object_count);
    println!("  {} traits", stats.trait_count);
    println!("  {} references", stats.ref_count);
    
    if error_count > 0 {
        println!("  {} errors", error_count);
    }
    
    Ok(())
}

/// List tasks
pub fn tasks(vault_path: &Path, status: Option<String>, due: Option<String>, all: bool) -> Result<()> {
    let db = Database::open(vault_path)?;
    
    let tasks = db.query_tasks(
        status.as_deref(),
        due.as_deref(),
        all,
    )?;
    
    if tasks.is_empty() {
        println!("No tasks found.");
        return Ok(());
    }
    
    for task in tasks {
        let fields: serde_json::Value = serde_json::from_str(&task.fields).unwrap_or_default();
        let status = fields.get("status").and_then(|v| v.as_str()).unwrap_or("todo");
        let due = fields.get("due").and_then(|v| v.as_str()).unwrap_or("-");
        let priority = fields.get("priority").and_then(|v| v.as_str()).unwrap_or("medium");
        
        let status_icon = match status {
            "todo" => "○",
            "in_progress" => "◐",
            "done" => "●",
            _ => "?",
        };
        
        let priority_color = match priority {
            "high" => "\x1b[31m",    // red
            "low" => "\x1b[90m",     // gray
            _ => "",
        };
        let reset = if priority_color.is_empty() { "" } else { "\x1b[0m" };
        
        println!("{} {}{}{}", status_icon, priority_color, task.content, reset);
        println!("  due: {} | {}", due, task.file_path);
    }
    
    Ok(())
}

/// Query traits
pub fn query_trait(vault_path: &Path, name: &str, filters: &[String]) -> Result<()> {
    let db = Database::open(vault_path)?;
    
    // For now, just handle task queries (can be extended for other traits)
    if name == "task" {
        let mut status = None;
        let mut due = None;
        
        for filter in filters {
            if let Some((key, value)) = filter.split_once('=') {
                match key.trim_start_matches("--") {
                    "status" => status = Some(value.to_string()),
                    "due" => due = Some(value.to_string()),
                    _ => {}
                }
            }
        }
        
        return tasks(vault_path, status, due, false);
    }
    
    println!("Trait query for '{}' not yet implemented.", name);
    Ok(())
}

/// Query objects
pub fn query(vault_path: &Path, query_str: &str) -> Result<()> {
    let db = Database::open(vault_path)?;
    let query = crate::index::QueryBuilder::parse(query_str);
    
    let (where_clause, params) = query.build_where_clause();
    
    println!("Query: {}", query_str);
    println!("(Full query support coming soon)");
    println!();
    
    // For now, show basic stats
    let stats = db.stats()?;
    println!("Index contains: {} objects, {} traits, {} refs", 
        stats.object_count, stats.trait_count, stats.ref_count);
    
    Ok(())
}

/// Show backlinks
pub fn backlinks(vault_path: &Path, target: &str) -> Result<()> {
    let db = Database::open(vault_path)?;
    let links = db.backlinks(target)?;
    
    if links.is_empty() {
        println!("No backlinks found for '{}'", target);
        return Ok(());
    }
    
    println!("Backlinks to '{}':\n", target);
    for link in links {
        let display = link.display_text.as_deref().unwrap_or(&link.source_id);
        println!("  ← {} ({}:{})", display, link.file_path, link.line.unwrap_or(0));
    }
    
    Ok(())
}

/// Show statistics
pub fn stats(vault_path: &Path) -> Result<()> {
    let db = Database::open(vault_path)?;
    let stats = db.stats()?;
    
    println!("Vault Statistics");
    println!("================");
    println!("Files:      {}", stats.file_count);
    println!("Objects:    {}", stats.object_count);
    println!("Traits:     {}", stats.trait_count);
    println!("References: {}", stats.ref_count);
    
    Ok(())
}

/// List untyped pages
pub fn untyped(vault_path: &Path) -> Result<()> {
    let db = Database::open(vault_path)?;
    let pages = db.untyped_pages()?;
    
    if pages.is_empty() {
        println!("All files have explicit types! ✓");
        return Ok(());
    }
    
    println!("Untyped pages (using 'page' fallback):\n");
    for page in pages {
        println!("  {}", page);
    }
    
    Ok(())
}

/// Open/create today's daily note
pub fn daily(vault_path: &Path) -> Result<()> {
    let today = chrono::Local::now().format("%Y-%m-%d").to_string();
    let daily_path = vault_path.join("daily").join(format!("{}.md", today));
    
    if !daily_path.exists() {
        // Create the daily note
        std::fs::create_dir_all(daily_path.parent().unwrap())?;
        
        let content = format!(
            "---\ntype: daily\ndate: {}\n---\n\n# {}\n\n",
            today,
            chrono::Local::now().format("%A, %B %d, %Y")
        );
        
        std::fs::write(&daily_path, content)?;
        println!("Created: {}", daily_path.display());
    } else {
        println!("Today's note: {}", daily_path.display());
    }
    
    // Try to open in editor
    if let Ok(editor) = std::env::var("EDITOR") {
        std::process::Command::new(editor)
            .arg(&daily_path)
            .spawn()
            .ok();
    }
    
    Ok(())
}

/// Create a new typed note
pub fn new_note(vault_path: &Path, type_name: &str, title: &str) -> Result<()> {
    let schema = load_schema(vault_path)?;
    
    // Check if type exists
    if !schema.types.contains_key(type_name) && type_name != "page" {
        println!("Warning: Type '{}' is not defined in schema.yaml", type_name);
    }
    
    // Generate filename from title
    let filename = slug::slugify(title);
    let file_path = vault_path.join(format!("{}.md", filename));
    
    if file_path.exists() {
        println!("Error: File already exists: {}", file_path.display());
        std::process::exit(1);
    }
    
    let content = format!(
        "---\ntype: {}\n---\n\n# {}\n\n",
        type_name,
        title
    );
    
    std::fs::write(&file_path, &content)?;
    println!("Created: {}", file_path.display());
    
    // Try to open in editor
    if let Ok(editor) = std::env::var("EDITOR") {
        std::process::Command::new(editor)
            .arg(&file_path)
            .spawn()
            .ok();
    }
    
    Ok(())
}
