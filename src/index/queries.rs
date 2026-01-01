//! Query builder for complex queries

use std::collections::HashMap;

/// A query builder for objects and traits
#[derive(Debug, Default)]
pub struct QueryBuilder {
    /// Type filter
    pub object_type: Option<String>,
    
    /// Trait type filter
    pub trait_type: Option<String>,
    
    /// Field filters
    pub field_filters: HashMap<String, String>,
    
    /// Parent type filter
    pub parent_type: Option<String>,
    
    /// Tag filter
    pub tag: Option<String>,
}

impl QueryBuilder {
    pub fn new() -> Self {
        Self::default()
    }
    
    /// Parse a query string like "type:meeting attendees:[[alice]]"
    pub fn parse(query: &str) -> Self {
        let mut builder = Self::new();
        
        for part in query.split_whitespace() {
            if let Some((key, value)) = part.split_once(':') {
                match key {
                    "type" => builder.object_type = Some(value.to_string()),
                    "trait" => builder.trait_type = Some(value.to_string()),
                    "tags" => builder.tag = Some(value.to_string()),
                    "parent.type" => builder.parent_type = Some(value.to_string()),
                    _ => {
                        builder.field_filters.insert(key.to_string(), value.to_string());
                    }
                }
            }
        }
        
        builder
    }
    
    /// Build SQL WHERE clause
    pub fn build_where_clause(&self) -> (String, Vec<String>) {
        let mut conditions = Vec::new();
        let mut params = Vec::new();
        
        if let Some(t) = &self.object_type {
            conditions.push("type = ?".to_string());
            params.push(t.clone());
        }
        
        if let Some(tag) = &self.tag {
            conditions.push("json_extract(fields, '$.tags') LIKE ?".to_string());
            params.push(format!("%\"{}%", tag));
        }
        
        for (field, value) in &self.field_filters {
            conditions.push(format!("json_extract(fields, '$.{}') = ?", field));
            params.push(value.clone());
        }
        
        let where_clause = if conditions.is_empty() {
            String::new()
        } else {
            format!("WHERE {}", conditions.join(" AND "))
        };
        
        (where_clause, params)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_parse_query() {
        let query = QueryBuilder::parse("type:meeting tags:planning");
        
        assert_eq!(query.object_type, Some("meeting".to_string()));
        assert_eq!(query.tag, Some("planning".to_string()));
    }
}
