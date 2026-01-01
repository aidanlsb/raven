//! SQLite database operations

use anyhow::{Context, Result};
use rusqlite::{params, Connection};
use std::path::Path;

use crate::parser::{ParsedDocument, ParsedObject, ParsedTrait, ParsedRef};

/// Database handle
pub struct Database {
    conn: Connection,
}

impl Database {
    /// Open or create the database
    pub fn open(vault_path: &Path) -> Result<Self> {
        let db_dir = vault_path.join(".raven");
        std::fs::create_dir_all(&db_dir)
            .with_context(|| format!("Failed to create .raven directory: {}", db_dir.display()))?;
        
        let db_path = db_dir.join("index.db");
        let conn = Connection::open(&db_path)
            .with_context(|| format!("Failed to open database: {}", db_path.display()))?;
        
        let db = Database { conn };
        db.initialize()?;
        
        Ok(db)
    }
    
    /// Open an in-memory database (for testing)
    #[cfg(test)]
    pub fn open_in_memory() -> Result<Self> {
        let conn = Connection::open_in_memory()?;
        let db = Database { conn };
        db.initialize()?;
        Ok(db)
    }
    
    /// Initialize the database schema
    fn initialize(&self) -> Result<()> {
        self.conn.execute_batch(r#"
            -- Enable WAL mode for better concurrency
            PRAGMA journal_mode = WAL;
            
            -- All referenceable objects (files + embedded types)
            CREATE TABLE IF NOT EXISTS objects (
                id TEXT PRIMARY KEY,
                file_path TEXT NOT NULL,
                type TEXT NOT NULL,
                heading TEXT,
                heading_level INTEGER,
                fields TEXT NOT NULL DEFAULT '{}',
                line_start INTEGER NOT NULL,
                line_end INTEGER,
                parent_id TEXT,
                created_at INTEGER,
                updated_at INTEGER
            );
            
            -- All trait annotations
            CREATE TABLE IF NOT EXISTS traits (
                id TEXT PRIMARY KEY,
                file_path TEXT NOT NULL,
                parent_object_id TEXT NOT NULL,
                trait_type TEXT NOT NULL,
                content TEXT NOT NULL,
                fields TEXT NOT NULL DEFAULT '{}',
                line_number INTEGER NOT NULL,
                created_at INTEGER
            );
            
            -- References between objects
            CREATE TABLE IF NOT EXISTS refs (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                source_id TEXT NOT NULL,
                target_id TEXT,
                target_raw TEXT NOT NULL,
                display_text TEXT,
                file_path TEXT NOT NULL,
                line_number INTEGER,
                position_start INTEGER,
                position_end INTEGER
            );
            
            -- Indexes for fast queries
            CREATE INDEX IF NOT EXISTS idx_objects_file ON objects(file_path);
            CREATE INDEX IF NOT EXISTS idx_objects_type ON objects(type);
            CREATE INDEX IF NOT EXISTS idx_objects_parent ON objects(parent_id);
            
            CREATE INDEX IF NOT EXISTS idx_traits_file ON traits(file_path);
            CREATE INDEX IF NOT EXISTS idx_traits_type ON traits(trait_type);
            CREATE INDEX IF NOT EXISTS idx_traits_parent ON traits(parent_object_id);
            
            CREATE INDEX IF NOT EXISTS idx_refs_source ON refs(source_id);
            CREATE INDEX IF NOT EXISTS idx_refs_target ON refs(target_id);
            CREATE INDEX IF NOT EXISTS idx_refs_file ON refs(file_path);
        "#).context("Failed to initialize database schema")?;
        
        Ok(())
    }
    
    /// Index a parsed document (replaces existing data for the file)
    pub fn index_document(&mut self, doc: &ParsedDocument) -> Result<()> {
        let tx = self.conn.transaction()?;
        
        // Delete existing data for this file
        tx.execute("DELETE FROM objects WHERE file_path = ?", params![&doc.file_path])?;
        tx.execute("DELETE FROM traits WHERE file_path = ?", params![&doc.file_path])?;
        tx.execute("DELETE FROM refs WHERE file_path = ?", params![&doc.file_path])?;
        
        let now = chrono::Utc::now().timestamp();
        
        // Insert objects
        for obj in &doc.objects {
            let fields_json = serde_json::to_string(&obj.fields)?;
            tx.execute(
                r#"INSERT INTO objects 
                   (id, file_path, type, heading, heading_level, fields, line_start, line_end, parent_id, created_at, updated_at)
                   VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"#,
                params![
                    &obj.id,
                    &doc.file_path,
                    &obj.object_type,
                    &obj.heading,
                    &obj.heading_level,
                    &fields_json,
                    &obj.line_start,
                    &obj.line_end,
                    &obj.parent_id,
                    now,
                    now,
                ],
            )?;
        }
        
        // Insert traits
        for (idx, trait_item) in doc.traits.iter().enumerate() {
            let trait_id = format!("{}:trait:{}", doc.file_path, idx);
            let fields_json = serde_json::to_string(&trait_item.fields)?;
            tx.execute(
                r#"INSERT INTO traits
                   (id, file_path, parent_object_id, trait_type, content, fields, line_number, created_at)
                   VALUES (?, ?, ?, ?, ?, ?, ?, ?)"#,
                params![
                    &trait_id,
                    &doc.file_path,
                    &trait_item.parent_object_id,
                    &trait_item.trait_type,
                    &trait_item.content,
                    &fields_json,
                    &trait_item.line,
                    now,
                ],
            )?;
        }
        
        // Insert refs
        for ref_item in &doc.refs {
            tx.execute(
                r#"INSERT INTO refs
                   (source_id, target_id, target_raw, display_text, file_path, line_number, position_start, position_end)
                   VALUES (?, ?, ?, ?, ?, ?, ?, ?)"#,
                params![
                    &ref_item.source_id,
                    Option::<String>::None, // target_id resolved later
                    &ref_item.target_raw,
                    &ref_item.display_text,
                    &doc.file_path,
                    &ref_item.line,
                    &ref_item.start,
                    &ref_item.end,
                ],
            )?;
        }
        
        tx.commit()?;
        Ok(())
    }
    
    /// Remove all data for a file
    pub fn remove_file(&mut self, file_path: &str) -> Result<()> {
        self.conn.execute("DELETE FROM objects WHERE file_path = ?", params![file_path])?;
        self.conn.execute("DELETE FROM traits WHERE file_path = ?", params![file_path])?;
        self.conn.execute("DELETE FROM refs WHERE file_path = ?", params![file_path])?;
        Ok(())
    }
    
    /// Get statistics about the index
    pub fn stats(&self) -> Result<IndexStats> {
        let object_count: i64 = self.conn.query_row(
            "SELECT COUNT(*) FROM objects",
            [],
            |row| row.get(0),
        )?;
        
        let trait_count: i64 = self.conn.query_row(
            "SELECT COUNT(*) FROM traits",
            [],
            |row| row.get(0),
        )?;
        
        let ref_count: i64 = self.conn.query_row(
            "SELECT COUNT(*) FROM refs",
            [],
            |row| row.get(0),
        )?;
        
        let file_count: i64 = self.conn.query_row(
            "SELECT COUNT(DISTINCT file_path) FROM objects",
            [],
            |row| row.get(0),
        )?;
        
        Ok(IndexStats {
            object_count: object_count as usize,
            trait_count: trait_count as usize,
            ref_count: ref_count as usize,
            file_count: file_count as usize,
        })
    }
    
    /// Query tasks with optional filters
    pub fn query_tasks(&self, status_filter: Option<&str>, _due_filter: Option<&str>, include_done: bool) -> Result<Vec<TaskResult>> {
        let mut sql = String::from(
            r#"SELECT t.id, t.content, t.fields, t.line_number, t.file_path, t.parent_object_id
               FROM traits t
               WHERE t.trait_type = 'task'"#
        );
        
        let mut conditions: Vec<String> = Vec::new();
        
        if !include_done {
            // Include tasks where status is null/missing OR status is not 'done'
            conditions.push("(json_extract(t.fields, '$.status') IS NULL OR json_extract(t.fields, '$.status') != 'done')".to_string());
        }
        
        if let Some(status) = status_filter {
            conditions.push(format!("json_extract(t.fields, '$.status') = '{}'", status));
        }
        
        // TODO: Add due date filtering
        
        if !conditions.is_empty() {
            sql.push_str(" AND ");
            sql.push_str(&conditions.join(" AND "));
        }
        
        sql.push_str(" ORDER BY json_extract(t.fields, '$.due') ASC");
        
        let mut stmt = self.conn.prepare(&sql)?;
        let tasks = stmt.query_map([], |row| {
            Ok(TaskResult {
                id: row.get(0)?,
                content: row.get(1)?,
                fields: row.get(2)?,
                line: row.get(3)?,
                file_path: row.get(4)?,
                parent_id: row.get(5)?,
            })
        })?;
        
        tasks.collect::<Result<Vec<_>, _>>().map_err(Into::into)
    }
    
    /// Get backlinks to a target
    pub fn backlinks(&self, target: &str) -> Result<Vec<BacklinkResult>> {
        let mut stmt = self.conn.prepare(
            r#"SELECT r.source_id, r.file_path, r.line_number, r.display_text
               FROM refs r
               WHERE r.target_raw = ? OR r.target_raw LIKE ?"#
        )?;
        
        // Match both exact and partial paths
        let partial = format!("%/{}", target);
        
        let results = stmt.query_map(params![target, partial], |row| {
            Ok(BacklinkResult {
                source_id: row.get(0)?,
                file_path: row.get(1)?,
                line: row.get(2)?,
                display_text: row.get(3)?,
            })
        })?;
        
        results.collect::<Result<Vec<_>, _>>().map_err(Into::into)
    }
    
    /// Get untyped pages (type = 'page')
    pub fn untyped_pages(&self) -> Result<Vec<String>> {
        let mut stmt = self.conn.prepare(
            "SELECT id FROM objects WHERE type = 'page' AND parent_id IS NULL"
        )?;
        
        let results = stmt.query_map([], |row| row.get(0))?;
        results.collect::<Result<Vec<_>, _>>().map_err(Into::into)
    }
    
    /// Get all object IDs (for reference resolution)
    pub fn all_object_ids(&self) -> Result<Vec<String>> {
        let mut stmt = self.conn.prepare("SELECT id FROM objects")?;
        let results = stmt.query_map([], |row| row.get(0))?;
        results.collect::<Result<Vec<_>, _>>().map_err(Into::into)
    }
}

/// Index statistics
#[derive(Debug)]
pub struct IndexStats {
    pub object_count: usize,
    pub trait_count: usize,
    pub ref_count: usize,
    pub file_count: usize,
}

/// Task query result
#[derive(Debug)]
pub struct TaskResult {
    pub id: String,
    pub content: String,
    pub fields: String,
    pub line: i64,
    pub file_path: String,
    pub parent_id: String,
}

/// Backlink query result
#[derive(Debug)]
pub struct BacklinkResult {
    pub source_id: String,
    pub file_path: String,
    pub line: Option<i64>,
    pub display_text: Option<String>,
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;
    
    #[test]
    fn test_database_initialization() {
        let db = Database::open_in_memory().unwrap();
        let stats = db.stats().unwrap();
        assert_eq!(stats.object_count, 0);
    }
    
    #[test]
    fn test_index_document() {
        let mut db = Database::open_in_memory().unwrap();
        
        let doc = ParsedDocument {
            file_path: "test.md".to_string(),
            objects: vec![
                ParsedObject {
                    id: "test".to_string(),
                    object_type: "page".to_string(),
                    fields: HashMap::new(),
                    tags: vec![],
                    heading: None,
                    heading_level: None,
                    parent_id: None,
                    line_start: 1,
                    line_end: None,
                }
            ],
            traits: vec![],
            refs: vec![],
        };
        
        db.index_document(&doc).unwrap();
        
        let stats = db.stats().unwrap();
        assert_eq!(stats.object_count, 1);
        assert_eq!(stats.file_count, 1);
    }
}
