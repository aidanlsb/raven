//! Trait annotation parser - parses @trait(...) syntax

use anyhow::Result;
use regex::Regex;
use std::collections::HashMap;

use crate::schema::FieldValue;

lazy_static::lazy_static! {
    // Matches @trait_name or @trait_name(args...) at word boundary
    // The (?:^|[\s\-\*]) ensures @ is at start of line or after whitespace/list markers
    static ref TRAIT_REGEX: Regex = Regex::new(
        r"(?:^|[\s\-\*])@(\w+)(?:\s*\(([^)]*)\))?"
    ).unwrap();
}

/// A parsed trait annotation
#[derive(Debug, Clone)]
pub struct TraitAnnotation {
    /// The trait name (e.g., "task", "remind")
    pub trait_name: String,
    
    /// Field values
    pub fields: HashMap<String, FieldValue>,
    
    /// The content associated with this trait (text on same line)
    pub content: String,
    
    /// Line number where the trait appears
    pub line: usize,
    
    /// Character offset where trait starts
    pub start_offset: usize,
    
    /// Character offset where trait ends
    pub end_offset: usize,
}

/// Parse all trait annotations from a line
pub fn parse_trait_annotation(line: &str, line_number: usize) -> Vec<TraitAnnotation> {
    let mut traits = Vec::new();
    
    for caps in TRAIT_REGEX.captures_iter(line) {
        let full_match = caps.get(0).unwrap();
        let trait_name = caps.get(1).unwrap().as_str().to_string();
        let args_str = caps.get(2).map(|m| m.as_str()).unwrap_or("");
        
        // Parse arguments
        let fields = parse_trait_arguments(args_str).unwrap_or_default();
        
        // Extract content (everything after the trait annotation on the same line)
        let after_trait = &line[full_match.end()..];
        let content = after_trait.trim().to_string();
        
        traits.push(TraitAnnotation {
            trait_name,
            fields,
            content,
            line: line_number,
            start_offset: full_match.start(),
            end_offset: full_match.end(),
        });
    }
    
    traits
}

/// Parse trait arguments (simplified version of type_decl parser)
fn parse_trait_arguments(args: &str) -> Result<HashMap<String, FieldValue>> {
    // Reuse the argument parsing logic from type_decl
    crate::parser::type_decl::parse_type_declaration(
        &format!("::dummy({})", args),
        0
    ).map(|opt| opt.map(|d| d.fields).unwrap_or_default())
}

/// Extract the full content for a trait (the paragraph/line containing it)
pub fn extract_trait_content(lines: &[&str], line_idx: usize) -> String {
    // For now, just return the content after the trait on the same line
    // A more sophisticated version would find paragraph boundaries
    if line_idx < lines.len() {
        let line = lines[line_idx];
        // Remove the trait annotation itself, return remaining content
        if let Some(result) = TRAIT_REGEX.replace_all(line, "").into_owned().trim().to_string().into() {
            return result;
        }
    }
    String::new()
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_parse_simple_trait() {
        let traits = parse_trait_annotation("- @task(due=2025-02-01) Send the email", 10);
        
        assert_eq!(traits.len(), 1);
        assert_eq!(traits[0].trait_name, "task");
        assert_eq!(traits[0].content, "Send the email");
        assert_eq!(traits[0].line, 10);
    }
    
    #[test]
    fn test_parse_trait_no_args() {
        let traits = parse_trait_annotation("- @highlight This is important", 1);
        
        assert_eq!(traits.len(), 1);
        assert_eq!(traits[0].trait_name, "highlight");
        assert!(traits[0].fields.is_empty());
    }
    
    #[test]
    fn test_parse_multiple_traits() {
        let traits = parse_trait_annotation("- @task(due=2025-02-01) @highlight Fix this bug", 1);
        
        assert_eq!(traits.len(), 2);
        assert_eq!(traits[0].trait_name, "task");
        assert_eq!(traits[1].trait_name, "highlight");
    }
    
    #[test]
    fn test_no_traits() {
        let traits = parse_trait_annotation("Just a regular line of text", 1);
        assert!(traits.is_empty());
    }
}
