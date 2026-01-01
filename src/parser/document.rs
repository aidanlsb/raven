//! Document parsing - combines all parser modules

use anyhow::{Context, Result};
use std::collections::HashMap;
use std::path::Path;

use crate::schema::FieldValue;
use super::frontmatter::parse_frontmatter;
use super::markdown::{extract_headings, extract_inline_tags, Heading};
use super::type_decl::parse_embedded_type;
use super::traits::parse_trait;
use super::refs::extract_refs;

/// A fully parsed document
#[derive(Debug, Clone)]
pub struct ParsedDocument {
    /// File path relative to vault
    pub file_path: String,
    
    /// All objects in this document
    pub objects: Vec<ParsedObject>,
    
    /// All traits in this document
    pub traits: Vec<ParsedTrait>,
    
    /// All references in this document
    pub refs: Vec<ParsedRef>,
}

/// A parsed object (file-level or embedded)
#[derive(Debug, Clone)]
pub struct ParsedObject {
    /// Unique ID (path for file-level, path#id for embedded)
    pub id: String,
    
    /// Type name
    pub object_type: String,
    
    /// Fields/metadata
    pub fields: HashMap<String, FieldValue>,
    
    /// Tags
    pub tags: Vec<String>,
    
    /// Heading text (for embedded objects)
    pub heading: Option<String>,
    
    /// Heading level (for embedded objects)
    pub heading_level: Option<u32>,
    
    /// Parent object ID (for embedded objects)
    pub parent_id: Option<String>,
    
    /// Line where this object starts
    pub line_start: usize,
    
    /// Line where this object ends
    pub line_end: Option<usize>,
}

/// A parsed trait annotation
#[derive(Debug, Clone)]
pub struct ParsedTrait {
    /// Trait type name (e.g., "task", "remind")
    pub trait_type: String,
    
    /// The content the trait annotates
    pub content: String,
    
    /// Trait fields
    pub fields: HashMap<String, FieldValue>,
    
    /// Parent object ID
    pub parent_object_id: String,
    
    /// Line number
    pub line: usize,
}

/// A parsed reference
#[derive(Debug, Clone)]
pub struct ParsedRef {
    /// Source object ID
    pub source_id: String,
    
    /// Raw target (as written)
    pub target_raw: String,
    
    /// Display text
    pub display_text: Option<String>,
    
    /// Line number
    pub line: usize,
    
    /// Start position
    pub start: usize,
    
    /// End position
    pub end: usize,
}

/// Parse a markdown document
pub fn parse_document(content: &str, file_path: &Path, vault_path: &Path) -> Result<ParsedDocument> {
    let relative_path = file_path
        .strip_prefix(vault_path)
        .unwrap_or(file_path)
        .to_string_lossy()
        .to_string();
    
    // File ID is path without extension
    let file_id = relative_path
        .strip_suffix(".md")
        .unwrap_or(&relative_path)
        .to_string();
    
    let mut objects = Vec::new();
    let mut traits = Vec::new();
    let mut refs = Vec::new();
    
    // Parse frontmatter
    let frontmatter = parse_frontmatter(content)
        .with_context(|| format!("Failed to parse frontmatter in {}", relative_path))?;
    
    let content_start_line = frontmatter.as_ref().map(|fm| fm.end_line + 1).unwrap_or(1);
    let body_content = if let Some(ref fm) = frontmatter {
        content.lines().skip(fm.end_line).collect::<Vec<_>>().join("\n")
    } else {
        content.to_string()
    };
    
    // Create file-level object
    let mut file_fields = frontmatter
        .as_ref()
        .map(|fm| fm.fields.clone())
        .unwrap_or_default();
    
    let file_type = frontmatter
        .as_ref()
        .and_then(|fm| fm.object_type.clone())
        .unwrap_or_else(|| "page".to_string());
    
    let mut file_tags = frontmatter
        .as_ref()
        .map(|fm| fm.tags.clone())
        .unwrap_or_default();
    
    // Add inline tags from body
    let inline_tags = extract_inline_tags(&body_content);
    for tag in inline_tags {
        if !file_tags.contains(&tag) {
            file_tags.push(tag);
        }
    }
    
    // Store tags in fields as well
    if !file_tags.is_empty() {
        file_fields.insert("tags".to_string(), FieldValue::Array(
            file_tags.iter().map(|t| FieldValue::String(t.clone())).collect()
        ));
    }
    
    objects.push(ParsedObject {
        id: file_id.clone(),
        object_type: file_type,
        fields: file_fields,
        tags: file_tags,
        heading: None,
        heading_level: None,
        parent_id: None,
        line_start: 1,
        line_end: None,
    });
    
    // Extract all headings from the body using pulldown-cmark
    let headings = extract_headings(&body_content, content_start_line);
    
    // Build a map of line -> type declaration for quick lookup
    let body_lines: Vec<&str> = body_content.lines().collect();
    let mut type_decl_lines: HashMap<usize, _> = HashMap::new();
    
    for (line_offset, line) in body_lines.iter().enumerate() {
        let line_num = content_start_line + line_offset;
        if let Some(embedded) = parse_embedded_type(line, line_num) {
            type_decl_lines.insert(line_num, embedded);
        }
    }
    
    // Track used IDs to ensure uniqueness
    let mut used_ids: HashMap<String, usize> = HashMap::new();
    
    // Process each heading - create either a typed object or a section
    // Also track parent hierarchy based on heading levels
    let mut parent_stack: Vec<(String, u32)> = vec![(file_id.clone(), 0)]; // (id, level)
    
    for heading in &headings {
        // Check if the line after this heading has a type declaration
        let next_line = heading.line + 1;
        
        // Pop parents that are at same or deeper level
        while parent_stack.len() > 1 && parent_stack.last().unwrap().1 >= heading.level {
            parent_stack.pop();
        }
        let current_parent = parent_stack.last().unwrap().0.clone();
        
        if let Some(embedded) = type_decl_lines.get(&next_line) {
            // Explicit type declaration - use the provided ID and type
            let embedded_id = format!("{}#{}", file_id, embedded.id);
            
            objects.push(ParsedObject {
                id: embedded_id.clone(),
                object_type: embedded.type_name.clone(),
                fields: embedded.fields.clone(),
                tags: embedded.tags.clone(),
                heading: Some(heading.text.clone()),
                heading_level: Some(heading.level),
                parent_id: Some(current_parent),
                line_start: heading.line,
                line_end: None,
            });
            
            parent_stack.push((embedded_id, heading.level));
        } else {
            // No type declaration - create a "section" object
            let base_slug = slug::slugify(&heading.text);
            let slug = if base_slug.is_empty() {
                format!("section-{}", heading.line)
            } else {
                base_slug
            };
            
            // Ensure unique ID by appending number if needed
            let unique_slug = {
                let count = used_ids.entry(slug.clone()).or_insert(0);
                *count += 1;
                if *count == 1 {
                    slug
                } else {
                    format!("{}-{}", slug, count)
                }
            };
            
            let section_id = format!("{}#{}", file_id, unique_slug);
            
            // Add title field
            let mut fields = HashMap::new();
            fields.insert("title".to_string(), FieldValue::String(heading.text.clone()));
            fields.insert("level".to_string(), FieldValue::Number(heading.level as f64));
            
            objects.push(ParsedObject {
                id: section_id.clone(),
                object_type: "section".to_string(),
                fields,
                tags: vec![],
                heading: Some(heading.text.clone()),
                heading_level: Some(heading.level),
                parent_id: Some(current_parent),
                line_start: heading.line,
                line_end: None,
            });
            
            parent_stack.push((section_id, heading.level));
        }
    }
    
    // Now process traits - assign to the correct parent based on line number
    for (line_offset, line) in body_lines.iter().enumerate() {
        let line_num = content_start_line + line_offset;
        
        if let Some(parsed_trait) = parse_trait(line, line_num) {
            // Find the parent object that contains this line
            let parent_id = find_parent_for_line(&objects, line_num);
            
            traits.push(ParsedTrait {
                trait_type: parsed_trait.name,
                content: parsed_trait.content,
                fields: parsed_trait.fields,
                parent_object_id: parent_id,
                line: line_num,
            });
        }
    }
    
    // Extract all references from body
    let body_refs = extract_refs(&body_content, content_start_line);
    for ref_item in body_refs {
        // Find parent for this reference
        let parent_id = find_parent_for_line(&objects, ref_item.line);
        
        refs.push(ParsedRef {
            source_id: parent_id,
            target_raw: ref_item.target_raw,
            display_text: ref_item.display_text,
            line: ref_item.line,
            start: ref_item.start,
            end: ref_item.end,
        });
    }
    
    // Compute line_end for each object
    compute_line_ends(&mut objects);
    
    Ok(ParsedDocument {
        file_path: relative_path,
        objects,
        traits,
        refs,
    })
}

/// Find the parent object ID for a given line number
fn find_parent_for_line(objects: &[ParsedObject], line: usize) -> String {
    // Find the object that starts at or before this line with the latest start
    objects.iter()
        .filter(|obj| obj.line_start <= line)
        .max_by_key(|obj| obj.line_start)
        .map(|obj| obj.id.clone())
        .unwrap_or_else(|| objects.first().map(|o| o.id.clone()).unwrap_or_default())
}

/// Compute line_end for each object based on the next object's line_start
fn compute_line_ends(objects: &mut [ParsedObject]) {
    // Sort by line_start
    let mut indices: Vec<usize> = (0..objects.len()).collect();
    indices.sort_by_key(|&i| objects[i].line_start);
    
    for i in 0..indices.len() {
        let current_idx = indices[i];
        let next_line_end = if i + 1 < indices.len() {
            Some(objects[indices[i + 1]].line_start - 1)
        } else {
            None // Last object extends to end of file
        };
        objects[current_idx].line_end = next_line_end;
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;
    
    #[test]
    fn test_parse_simple_document() {
        let content = r#"---
type: person
name: Alice
---

# Alice

Some content about Alice.
"#;
        let file_path = PathBuf::from("/vault/people/alice.md");
        let vault_path = PathBuf::from("/vault");
        
        let doc = parse_document(content, &file_path, &vault_path).unwrap();
        
        assert_eq!(doc.file_path, "people/alice.md");
        // Should have file-level object + section for "# Alice"
        assert_eq!(doc.objects.len(), 2);
        assert_eq!(doc.objects[0].id, "people/alice");
        assert_eq!(doc.objects[0].object_type, "person");
        assert_eq!(doc.objects[1].object_type, "section");
        assert_eq!(doc.objects[1].heading, Some("Alice".to_string()));
    }
    
    #[test]
    fn test_parse_document_with_sections() {
        let content = r#"# Introduction

Some intro text.

## Background

More text here.

## Methods

Even more text.
"#;
        let file_path = PathBuf::from("/vault/doc.md");
        let vault_path = PathBuf::from("/vault");
        
        let doc = parse_document(content, &file_path, &vault_path).unwrap();
        
        // File + 3 sections
        assert_eq!(doc.objects.len(), 4);
        assert_eq!(doc.objects[0].object_type, "page");
        assert_eq!(doc.objects[1].object_type, "section");
        assert_eq!(doc.objects[1].id, "doc#introduction");
        assert_eq!(doc.objects[2].object_type, "section");
        assert_eq!(doc.objects[2].id, "doc#background");
        assert_eq!(doc.objects[3].object_type, "section");
        assert_eq!(doc.objects[3].id, "doc#methods");
    }
    
    #[test]
    fn test_explicit_type_overrides_section() {
        let content = r#"# Weekly Standup
::meeting(id=standup, time=09:00)

Discussion notes here.
"#;
        let file_path = PathBuf::from("/vault/meetings.md");
        let vault_path = PathBuf::from("/vault");
        
        let doc = parse_document(content, &file_path, &vault_path).unwrap();
        
        // File + 1 meeting (not section)
        assert_eq!(doc.objects.len(), 2);
        assert_eq!(doc.objects[1].object_type, "meeting");
        assert_eq!(doc.objects[1].id, "meetings#standup");
    }
    
    #[test]
    fn test_duplicate_heading_ids() {
        let content = r#"# Notes

## Ideas

First ideas section.

## Ideas

Second ideas section with same heading.
"#;
        let file_path = PathBuf::from("/vault/doc.md");
        let vault_path = PathBuf::from("/vault");
        
        let doc = parse_document(content, &file_path, &vault_path).unwrap();
        
        // Should have unique IDs
        assert_eq!(doc.objects.len(), 4);
        assert_eq!(doc.objects[2].id, "doc#ideas");
        assert_eq!(doc.objects[3].id, "doc#ideas-2");
    }
    
    #[test]
    fn test_trait_parented_to_section() {
        let content = r#"# Project

## Tasks

- @task(due=2024-01-15) Do the thing
"#;
        let file_path = PathBuf::from("/vault/project.md");
        let vault_path = PathBuf::from("/vault");
        
        let doc = parse_document(content, &file_path, &vault_path).unwrap();
        
        assert_eq!(doc.traits.len(), 1);
        // Task should be parented to the "Tasks" section
        assert_eq!(doc.traits[0].parent_object_id, "project#tasks");
    }
}
