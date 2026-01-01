//! Reference parsing ([[wikilinks]])

use regex::Regex;
use lazy_static::lazy_static;

/// A parsed reference
#[derive(Debug, Clone)]
pub struct Reference {
    /// The raw target (as written)
    pub target_raw: String,
    
    /// Display text (if different from target)
    pub display_text: Option<String>,
    
    /// Line number where found
    pub line: usize,
    
    /// Start position in line
    pub start: usize,
    
    /// End position in line
    pub end: usize,
}

lazy_static! {
    // Match [[target]] or [[target|display]]
    // The target cannot contain [ or ] to avoid matching array syntax like [[[ref]]]
    static ref WIKILINK_RE: Regex = Regex::new(r"\[\[([^\]\[|]+)(?:\|([^\]]+))?\]\]").unwrap();
}

/// Extract references from content
pub fn extract_refs(content: &str, start_line: usize) -> Vec<Reference> {
    let mut refs = Vec::new();
    
    for (line_offset, line) in content.lines().enumerate() {
        let line_num = start_line + line_offset;
        
        for cap in WIKILINK_RE.captures_iter(line) {
            let full_match = cap.get(0).unwrap();
            
            // Skip if preceded by [ (array syntax like [[[ref]]])
            if full_match.start() > 0 {
                let prev_char = line.chars().nth(full_match.start() - 1);
                if prev_char == Some('[') {
                    continue;
                }
            }
            
            let target = cap.get(1).unwrap().as_str().trim();
            let display = cap.get(2).map(|m| m.as_str().trim().to_string());
            
            refs.push(Reference {
                target_raw: target.to_string(),
                display_text: display,
                line: line_num,
                start: full_match.start(),
                end: full_match.end(),
            });
        }
    }
    
    refs
}

/// Parse embedded refs in trait values like [[[path/to/file]], [[other]]]
/// Handles array syntax where refs are wrapped in extra brackets
pub fn extract_embedded_refs(value: &str) -> Vec<String> {
    let mut refs = Vec::new();
    
    for cap in WIKILINK_RE.captures_iter(value) {
        let full_match = cap.get(0).unwrap();
        let target = cap.get(1).unwrap().as_str().trim().to_string();
        
        // For embedded refs, we DO want to match inside arrays
        // Just make sure the target doesn't start with [
        if !target.starts_with('[') {
            refs.push(target);
        }
    }
    
    refs
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_extract_refs() {
        let content = "Check out [[some/file]] and [[another|Display Text]]";
        let refs = extract_refs(content, 1);
        
        assert_eq!(refs.len(), 2);
        assert_eq!(refs[0].target_raw, "some/file");
        assert_eq!(refs[0].display_text, None);
        assert_eq!(refs[1].target_raw, "another");
        assert_eq!(refs[1].display_text, Some("Display Text".to_string()));
    }
    
    #[test]
    fn test_extract_embedded_refs() {
        let value = "attendees=[[[alice]], [[bob]]]";
        let refs = extract_embedded_refs(value);
        
        assert_eq!(refs, vec!["alice", "bob"]);
    }
}
