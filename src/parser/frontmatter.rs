//! Frontmatter parser - extracts YAML frontmatter from markdown files

use anyhow::{Context, Result};
use std::collections::HashMap;

use crate::schema::FieldValue;

/// Parsed frontmatter data
#[derive(Debug, Clone, Default)]
pub struct Frontmatter {
    /// The declared type (if any)
    pub object_type: Option<String>,
    
    /// All fields from frontmatter
    pub fields: HashMap<String, FieldValue>,
    
    /// Tags declared in frontmatter
    pub tags: Vec<String>,
    
    /// Line number where frontmatter ends (0 if no frontmatter)
    pub end_line: usize,
}

/// Parse YAML frontmatter from markdown content
pub fn parse_frontmatter(content: &str) -> Result<Frontmatter> {
    let lines: Vec<&str> = content.lines().collect();
    
    // Check for frontmatter start
    if lines.is_empty() || lines[0].trim() != "---" {
        return Ok(Frontmatter::default());
    }
    
    // Find frontmatter end
    let mut end_idx = None;
    for (i, line) in lines.iter().enumerate().skip(1) {
        if line.trim() == "---" {
            end_idx = Some(i);
            break;
        }
    }
    
    let end_idx = match end_idx {
        Some(idx) => idx,
        None => return Ok(Frontmatter::default()), // No closing ---
    };
    
    // Extract frontmatter YAML
    let yaml_content: String = lines[1..end_idx].join("\n");
    
    // Parse YAML
    let yaml_value: serde_yaml::Value = serde_yaml::from_str(&yaml_content)
        .with_context(|| "Failed to parse frontmatter YAML")?;
    
    let mut frontmatter = Frontmatter {
        end_line: end_idx + 1, // 1-indexed
        ..Default::default()
    };
    
    if let serde_yaml::Value::Mapping(map) = yaml_value {
        for (key, value) in map {
            if let serde_yaml::Value::String(key_str) = key {
                match key_str.as_str() {
                    "type" => {
                        if let serde_yaml::Value::String(t) = value {
                            frontmatter.object_type = Some(t);
                        }
                    }
                    "tags" => {
                        frontmatter.tags = parse_tags_value(&value);
                    }
                    _ => {
                        frontmatter.fields.insert(key_str, yaml_to_field_value(&value));
                    }
                }
            }
        }
    }
    
    Ok(frontmatter)
}

/// Convert a YAML value to a FieldValue
fn yaml_to_field_value(value: &serde_yaml::Value) -> FieldValue {
    match value {
        serde_yaml::Value::String(s) => {
            // Check if it's a reference [[...]]
            if s.starts_with("[[") && s.ends_with("]]") {
                FieldValue::Ref(s[2..s.len()-2].to_string())
            } else if is_date_string(s) {
                FieldValue::Date(s.clone())
            } else if is_datetime_string(s) {
                FieldValue::Datetime(s.clone())
            } else {
                FieldValue::String(s.clone())
            }
        }
        serde_yaml::Value::Number(n) => {
            FieldValue::Number(n.as_f64().unwrap_or(0.0))
        }
        serde_yaml::Value::Bool(b) => FieldValue::Bool(*b),
        serde_yaml::Value::Sequence(arr) => {
            FieldValue::Array(arr.iter().map(yaml_to_field_value).collect())
        }
        serde_yaml::Value::Null => FieldValue::Null,
        _ => FieldValue::Null,
    }
}

/// Parse tags from a YAML value (can be string or array)
fn parse_tags_value(value: &serde_yaml::Value) -> Vec<String> {
    match value {
        serde_yaml::Value::Sequence(arr) => {
            arr.iter()
                .filter_map(|v| {
                    if let serde_yaml::Value::String(s) = v {
                        Some(s.clone())
                    } else {
                        None
                    }
                })
                .collect()
        }
        serde_yaml::Value::String(s) => {
            // Single tag
            vec![s.clone()]
        }
        _ => Vec::new(),
    }
}

/// Check if a string looks like an ISO date (YYYY-MM-DD)
fn is_date_string(s: &str) -> bool {
    if s.len() != 10 {
        return false;
    }
    let parts: Vec<&str> = s.split('-').collect();
    if parts.len() != 3 {
        return false;
    }
    parts[0].len() == 4 && parts[1].len() == 2 && parts[2].len() == 2
        && parts.iter().all(|p| p.chars().all(|c| c.is_ascii_digit()))
}

/// Check if a string looks like an ISO datetime
fn is_datetime_string(s: &str) -> bool {
    s.contains('T') && s.len() >= 16
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_parse_frontmatter() {
        let content = r#"---
type: person
name: Alice Chen
email: alice@example.com
---

# Alice Chen
"#;
        let fm = parse_frontmatter(content).unwrap();
        
        assert_eq!(fm.object_type, Some("person".to_string()));
        assert_eq!(fm.fields.get("name"), Some(&FieldValue::String("Alice Chen".to_string())));
        assert_eq!(fm.end_line, 5);
    }
    
    #[test]
    fn test_parse_frontmatter_with_tags() {
        let content = r#"---
type: daily
date: 2025-02-01
tags: [work, productivity]
---

# Content
"#;
        let fm = parse_frontmatter(content).unwrap();
        
        assert_eq!(fm.object_type, Some("daily".to_string()));
        assert_eq!(fm.tags, vec!["work", "productivity"]);
    }
    
    #[test]
    fn test_no_frontmatter() {
        let content = "# Just a heading\n\nSome content";
        let fm = parse_frontmatter(content).unwrap();
        
        assert!(fm.object_type.is_none());
        assert_eq!(fm.end_line, 0);
    }
}
