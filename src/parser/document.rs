//! Document parser - combines all parsers into a complete document representation

use anyhow::{Context, Result};
use std::collections::HashMap;
use std::path::Path;

use crate::schema::FieldValue;
use super::frontmatter::{parse_frontmatter, Frontmatter};
use super::markdown::{parse_markdown_structure, find_heading_scope, Heading};
use super::type_decl::{parse_type_declaration, TypeDeclaration};
use super::traits::{parse_trait_annotation, TraitAnnotation};
use super::refs::{extract_references, extract_tags, Reference, Tag};

/// A parsed object (file-level or embedded)
#[derive(Debug, Clone)]
pub struct ParsedObject {
    /// Object ID (file path for file-level, path#id for embedded)
    pub id: String,
    
    /// Object type
    pub object_type: String,
    
    /// Field values
    pub fields: HashMap<String, FieldValue>,
    
    /// Tags aggregated from content
    pub tags: Vec<String>,
    
    /// Heading text (None for file-level)
    pub heading: Option<String>,
    
    /// Heading level (None for file-level)
    pub heading_level: Option<u8>,
    
    /// Parent object ID (None for file-level)
    pub parent_id: Option<String>,
    
    /// Line number where object starts
    pub line_start: usize,
    
    /// Line number where object ends (for embedded objects)
    pub line_end: Option<usize>,
}

/// A parsed trait instance
#[derive(Debug, Clone)]
pub struct ParsedTrait {
    /// Trait type name
    pub trait_type: String,
    
    /// Parent object ID
    pub parent_object_id: String,
    
    /// Field values
    pub fields: HashMap<String, FieldValue>,
    
    /// Content associated with the trait
    pub content: String,
    
    /// Line number
    pub line: usize,
}

/// A parsed reference
#[derive(Debug, Clone)]
pub struct ParsedRef {
    /// Source object or trait ID
    pub source_id: String,
    
    /// Target (raw, may need resolution)
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

/// A fully parsed document
#[derive(Debug, Clone)]
pub struct ParsedDocument {
    /// File path (relative to vault)
    pub file_path: String,
    
    /// All objects in the document
    pub objects: Vec<ParsedObject>,
    
    /// All traits in the document
    pub traits: Vec<ParsedTrait>,
    
    /// All references in the document
    pub refs: Vec<ParsedRef>,
}

/// Parse a document from file content
pub fn parse_document(
    content: &str,
    file_path: &Path,
    vault_path: &Path,
) -> Result<ParsedDocument> {
    let relative_path = file_path
        .strip_prefix(vault_path)
        .unwrap_or(file_path)
        .to_string_lossy()
        .to_string();
    
    // Remove .md extension for ID
    let file_id = relative_path
        .strip_suffix(".md")
        .unwrap_or(&relative_path)
        .to_string();
    
    let lines: Vec<&str> = content.lines().collect();
    let total_lines = lines.len();
    
    // Parse frontmatter
    let frontmatter = parse_frontmatter(content)
        .with_context(|| format!("Failed to parse frontmatter in {}", relative_path))?;
    
    // Get content after frontmatter
    let content_start_line = if frontmatter.end_line > 0 {
        frontmatter.end_line + 1
    } else {
        1
    };
    
    let body_content = if frontmatter.end_line > 0 && frontmatter.end_line < lines.len() {
        lines[frontmatter.end_line..].join("\n")
    } else {
        content.to_string()
    };
    
    // Parse markdown structure
    let structure = parse_markdown_structure(&body_content, content_start_line);
    
    let mut objects = Vec::new();
    let mut all_traits = Vec::new();
    let mut all_refs = Vec::new();
    
    // Create file-level object
    let file_type = frontmatter.object_type.clone().unwrap_or_else(|| "page".to_string());
    let mut file_tags = frontmatter.tags.clone();
    
    // Find embedded objects and their scopes
    let mut embedded_objects: Vec<(usize, TypeDeclaration, Heading)> = Vec::new();
    
    for (heading_idx, heading) in structure.headings.iter().enumerate() {
        // Check the lines after the heading for a type declaration
        let heading_line_idx = heading.line.saturating_sub(1);
        for offset in 1..=2 {
            let check_idx = heading_line_idx + offset;
            if check_idx < lines.len() {
                if let Ok(Some(type_decl)) = parse_type_declaration(lines[check_idx], check_idx + 1) {
                    embedded_objects.push((heading_idx, type_decl, heading.clone()));
                    break;
                }
            }
        }
    }
    
    // Build embedded objects with proper scopes
    let mut embedded_scopes: Vec<(usize, usize, String)> = Vec::new(); // (start, end, object_id)
    
    for (heading_idx, type_decl, heading) in &embedded_objects {
        let (start, end) = find_heading_scope(&structure.headings, *heading_idx, total_lines);
        
        let embedded_id = type_decl.id.clone().unwrap_or_else(|| {
            // Generate ID from heading if not provided
            slug::slugify(&heading.text)
        });
        
        let full_id = format!("{}#{}", file_id, embedded_id);
        
        // Find parent
        let parent_id = find_parent_object(
            &structure.headings,
            *heading_idx,
            &embedded_objects,
            &file_id,
        );
        
        // Extract tags from this section
        let section_content = lines[start.saturating_sub(1)..end.min(total_lines)]
            .join("\n");
        let section_tags: Vec<String> = extract_tags(&section_content, start)
            .iter()
            .map(|t| t.name.clone())
            .collect();
        
        let mut fields = type_decl.fields.clone();
        if !section_tags.is_empty() {
            fields.insert(
                "tags".to_string(),
                FieldValue::Array(section_tags.iter().map(|t| FieldValue::String(t.clone())).collect()),
            );
        }
        
        objects.push(ParsedObject {
            id: full_id.clone(),
            object_type: type_decl.type_name.clone(),
            fields,
            tags: section_tags.clone(),
            heading: Some(heading.text.clone()),
            heading_level: Some(heading.level),
            parent_id: Some(parent_id),
            line_start: start,
            line_end: Some(end),
        });
        
        embedded_scopes.push((start, end, full_id));
    }
    
    // Extract tags from file-level content (outside embedded objects)
    for (line_idx, line) in lines.iter().enumerate() {
        let line_num = line_idx + 1;
        
        // Skip if this line is inside an embedded object
        let in_embedded = embedded_scopes.iter().any(|(start, end, _)| {
            line_num >= *start && line_num < *end
        });
        
        if !in_embedded {
            for tag in extract_tags(line, line_num) {
                if !file_tags.contains(&tag.name) {
                    file_tags.push(tag.name);
                }
            }
        }
    }
    
    // Inherit tags from children to file-level object
    for obj in &objects {
        for tag in &obj.tags {
            if !file_tags.contains(tag) {
                file_tags.push(tag.clone());
            }
        }
    }
    
    // Create file-level object
    let mut file_fields = frontmatter.fields.clone();
    if !file_tags.is_empty() {
        file_fields.insert(
            "tags".to_string(),
            FieldValue::Array(file_tags.iter().map(|t| FieldValue::String(t.clone())).collect()),
        );
    }
    
    objects.insert(0, ParsedObject {
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
    
    // Parse traits from all lines
    for (line_idx, line) in lines.iter().enumerate() {
        let line_num = line_idx + 1;
        
        let trait_annotations = parse_trait_annotation(line, line_num);
        
        for annotation in trait_annotations {
            // Find parent object
            let parent_id = find_trait_parent(&embedded_scopes, line_num, &file_id);
            
            all_traits.push(ParsedTrait {
                trait_type: annotation.trait_name,
                parent_object_id: parent_id,
                fields: annotation.fields,
                content: annotation.content,
                line: line_num,
            });
        }
    }
    
    // Extract references from all content
    let refs = extract_references(content, 1);
    for reference in refs {
        // Find source object
        let source_id = find_trait_parent(&embedded_scopes, reference.line, &file_id);
        
        all_refs.push(ParsedRef {
            source_id,
            target_raw: reference.target,
            display_text: reference.display_text,
            line: reference.line,
            start: reference.start,
            end: reference.end,
        });
    }
    
    Ok(ParsedDocument {
        file_path: relative_path,
        objects,
        traits: all_traits,
        refs: all_refs,
    })
}

/// Find the parent object for an embedded heading
fn find_parent_object(
    headings: &[Heading],
    heading_idx: usize,
    embedded_objects: &[(usize, TypeDeclaration, Heading)],
    file_id: &str,
) -> String {
    let current = &headings[heading_idx];
    
    // Look for nearest ancestor with lower level that is also an embedded object
    for i in (0..heading_idx).rev() {
        let ancestor = &headings[i];
        if ancestor.level < current.level {
            // Check if this ancestor is an embedded object
            for (idx, type_decl, _) in embedded_objects {
                if *idx == i {
                    let embedded_id = type_decl.id.clone().unwrap_or_else(|| {
                        slug::slugify(&ancestor.text)
                    });
                    return format!("{}#{}", file_id, embedded_id);
                }
            }
        }
    }
    
    // No embedded parent found, parent is the file
    file_id.to_string()
}

/// Find the parent object for a trait on a given line
fn find_trait_parent(
    embedded_scopes: &[(usize, usize, String)],
    line_num: usize,
    file_id: &str,
) -> String {
    // Find the most specific (deepest nested) embedded object containing this line
    for (start, end, id) in embedded_scopes.iter().rev() {
        if line_num >= *start && line_num < *end {
            return id.clone();
        }
    }
    file_id.to_string()
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;
    
    #[test]
    fn test_parse_simple_document() {
        let content = r#"---
type: daily
date: 2025-02-01
---

# February 1, 2025

Some content here.

- @task(due=2025-02-03) Send email
"#;
        
        let file_path = PathBuf::from("/vault/daily/2025-02-01.md");
        let vault_path = PathBuf::from("/vault");
        
        let doc = parse_document(content, &file_path, &vault_path).unwrap();
        
        assert_eq!(doc.file_path, "daily/2025-02-01.md");
        assert_eq!(doc.objects.len(), 1);
        assert_eq!(doc.objects[0].id, "daily/2025-02-01");
        assert_eq!(doc.objects[0].object_type, "daily");
        assert_eq!(doc.traits.len(), 1);
        assert_eq!(doc.traits[0].trait_type, "task");
    }
    
    #[test]
    fn test_parse_document_with_embedded() {
        let content = r#"---
type: daily
date: 2025-02-01
---

# February 1, 2025

## Weekly Standup
::meeting(id=standup, time=09:00)

Discussed roadmap.

- @task(due=2025-02-03) Follow up

## Reading

Just reading notes.
"#;
        
        let file_path = PathBuf::from("/vault/daily/2025-02-01.md");
        let vault_path = PathBuf::from("/vault");
        
        let doc = parse_document(content, &file_path, &vault_path).unwrap();
        
        assert_eq!(doc.objects.len(), 2);
        assert_eq!(doc.objects[0].id, "daily/2025-02-01");
        assert_eq!(doc.objects[1].id, "daily/2025-02-01#standup");
        assert_eq!(doc.objects[1].object_type, "meeting");
        assert_eq!(doc.objects[1].parent_id, Some("daily/2025-02-01".to_string()));
    }
}
