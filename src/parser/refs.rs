//! Reference and tag extractor

use regex::Regex;

lazy_static::lazy_static! {
    // Matches [[ref]] or [[ref|display text]]
    static ref REF_REGEX: Regex = Regex::new(
        r"\[\[([^\]|]+)(?:\|([^\]]+))?\]\]"
    ).unwrap();
    
    // Matches #tag (word characters and hyphens)
    static ref TAG_REGEX: Regex = Regex::new(
        r"#([\w\-]+)"
    ).unwrap();
}

/// A reference to another object
#[derive(Debug, Clone)]
pub struct Reference {
    /// The target (path or short name)
    pub target: String,
    
    /// Optional display text
    pub display_text: Option<String>,
    
    /// Line number
    pub line: usize,
    
    /// Start position in line
    pub start: usize,
    
    /// End position in line
    pub end: usize,
}

/// A tag found in content
#[derive(Debug, Clone)]
pub struct Tag {
    /// The tag name (without #)
    pub name: String,
    
    /// Line number
    pub line: usize,
}

/// Extract all references from content
pub fn extract_references(content: &str, start_line: usize) -> Vec<Reference> {
    let mut refs = Vec::new();
    
    for (line_idx, line) in content.lines().enumerate() {
        let line_num = start_line + line_idx;
        
        for caps in REF_REGEX.captures_iter(line) {
            let full_match = caps.get(0).unwrap();
            let target = caps.get(1).unwrap().as_str().to_string();
            let display_text = caps.get(2).map(|m| m.as_str().to_string());
            
            refs.push(Reference {
                target,
                display_text,
                line: line_num,
                start: full_match.start(),
                end: full_match.end(),
            });
        }
    }
    
    refs
}

/// Extract all tags from content
pub fn extract_tags(content: &str, start_line: usize) -> Vec<Tag> {
    let mut tags = Vec::new();
    
    for (line_idx, line) in content.lines().enumerate() {
        let line_num = start_line + line_idx;
        
        for caps in TAG_REGEX.captures_iter(line) {
            let name = caps.get(1).unwrap().as_str().to_string();
            tags.push(Tag { name, line: line_num });
        }
    }
    
    tags
}

/// Check if a target looks like a full path (contains /)
pub fn is_full_path(target: &str) -> bool {
    target.contains('/')
}

/// Extract the short name from a path
pub fn short_name(target: &str) -> &str {
    target.rsplit('/').next().unwrap_or(target)
}

/// Check if a target references an embedded object (contains #)
pub fn is_embedded_ref(target: &str) -> bool {
    target.contains('#')
}

/// Split an embedded reference into (file_path, embedded_id)
pub fn split_embedded_ref(target: &str) -> Option<(&str, &str)> {
    target.split_once('#')
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_extract_references() {
        let content = "Met with [[people/alice]] about [[projects/website]].";
        let refs = extract_references(content, 1);
        
        assert_eq!(refs.len(), 2);
        assert_eq!(refs[0].target, "people/alice");
        assert_eq!(refs[1].target, "projects/website");
    }
    
    #[test]
    fn test_extract_reference_with_display() {
        let content = "See [[people/alice|Alice Chen]] for details.";
        let refs = extract_references(content, 1);
        
        assert_eq!(refs.len(), 1);
        assert_eq!(refs[0].target, "people/alice");
        assert_eq!(refs[0].display_text, Some("Alice Chen".to_string()));
    }
    
    #[test]
    fn test_extract_embedded_ref() {
        let content = "See [[daily/2025-02-01#standup]] for notes.";
        let refs = extract_references(content, 1);
        
        assert_eq!(refs.len(), 1);
        assert!(is_embedded_ref(&refs[0].target));
        let (path, id) = split_embedded_ref(&refs[0].target).unwrap();
        assert_eq!(path, "daily/2025-02-01");
        assert_eq!(id, "standup");
    }
    
    #[test]
    fn test_extract_tags() {
        let content = "Some thoughts about #productivity and #habits today.";
        let tags = extract_tags(content, 1);
        
        assert_eq!(tags.len(), 2);
        assert_eq!(tags[0].name, "productivity");
        assert_eq!(tags[1].name, "habits");
    }
}
