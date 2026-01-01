//! Frontmatter parsing (YAML at the start of a markdown file)

use anyhow::{Context, Result};
use std::collections::HashMap;

use crate::schema::FieldValue;

/// Parsed frontmatter data
#[derive(Debug, Default, Clone)]
pub struct Frontmatter {
    /// The type field (if present)
    pub object_type: Option<String>,
    
    /// Tags from frontmatter
    pub tags: Vec<String>,
    
    /// All other fields
    pub fields: HashMap<String, FieldValue>,
    
    /// Raw frontmatter content
    pub raw: String,
    
    /// Line where frontmatter ends
    pub end_line: usize,
}

/// Parse YAML frontmatter from markdown content
pub fn parse_frontmatter(content: &str) -> Result<Option<Frontmatter>> {
    let lines: Vec<&str> = content.lines().collect();
    
    // Check for opening ---
    if lines.is_empty() || lines[0].trim() != "---" {
        return Ok(None);
    }
    
    // Find closing ---
    let end_line = lines[1..]
        .iter()
        .position(|line| line.trim() == "---")
        .map(|pos| pos + 1);
    
    let end_line = match end_line {
        Some(line) => line,
        None => return Ok(None), // No closing ---
    };
    
    // Extract frontmatter content
    let frontmatter_content: String = lines[1..end_line].join("\n");
    
    // Parse as YAML
    let yaml: serde_yaml::Value = serde_yaml::from_str(&frontmatter_content)
        .with_context(|| "Failed to parse frontmatter as YAML")?;
    
    let yaml_map = match yaml.as_mapping() {
        Some(m) => m,
        None => return Ok(None),
    };
    
    let mut frontmatter = Frontmatter {
        raw: frontmatter_content,
        end_line: end_line + 1, // +1 for 1-indexed lines
        ..Default::default()
    };
    
    for (key, value) in yaml_map {
        let key_str = match key.as_str() {
            Some(k) => k,
            None => continue,
        };
        
        match key_str {
            "type" => {
                frontmatter.object_type = value.as_str().map(String::from);
            }
            "tags" => {
                frontmatter.tags = parse_tags_value(value);
            }
            _ => {
                if let Some(field_value) = yaml_to_field_value(value) {
                    frontmatter.fields.insert(key_str.to_string(), field_value);
                }
            }
        }
    }
    
    Ok(Some(frontmatter))
}

/// Parse tags from various YAML formats
fn parse_tags_value(value: &serde_yaml::Value) -> Vec<String> {
    match value {
        serde_yaml::Value::String(s) => {
            // Single tag or space/comma separated
            s.split(|c| c == ',' || c == ' ')
                .map(|t| t.trim().trim_start_matches('#').to_string())
                .filter(|t| !t.is_empty())
                .collect()
        }
        serde_yaml::Value::Sequence(seq) => {
            seq.iter()
                .filter_map(|v| v.as_str())
                .map(|s| s.trim().trim_start_matches('#').to_string())
                .collect()
        }
        _ => vec![],
    }
}

/// Convert YAML value to FieldValue
fn yaml_to_field_value(value: &serde_yaml::Value) -> Option<FieldValue> {
    match value {
        serde_yaml::Value::String(s) => Some(FieldValue::String(s.clone())),
        serde_yaml::Value::Number(n) => {
            if let Some(i) = n.as_i64() {
                Some(FieldValue::Number(i as f64))
            } else if let Some(f) = n.as_f64() {
                Some(FieldValue::Number(f))
            } else {
                None
            }
        }
        serde_yaml::Value::Bool(b) => Some(FieldValue::Bool(*b)),
        serde_yaml::Value::Sequence(seq) => {
            let items: Vec<FieldValue> = seq.iter()
                .filter_map(yaml_to_field_value)
                .collect();
            Some(FieldValue::Array(items))
        }
        serde_yaml::Value::Null => Some(FieldValue::Null),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_parse_frontmatter() {
        let content = r#"---
type: person
name: Alice
tags: [friend, colleague]
---

# Alice

Some content
"#;
        
        let fm = parse_frontmatter(content).unwrap().unwrap();
        
        assert_eq!(fm.object_type, Some("person".to_string()));
        assert_eq!(fm.tags, vec!["friend", "colleague"]);
        assert!(fm.fields.contains_key("name"));
    }
    
    #[test]
    fn test_no_frontmatter() {
        let content = "# Just a heading\n\nSome content";
        let fm = parse_frontmatter(content).unwrap();
        assert!(fm.is_none());
    }
}
