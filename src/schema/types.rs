//! Schema type definitions

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// The complete schema definition loaded from schema.yaml
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Schema {
    #[serde(default)]
    pub types: HashMap<String, TypeDefinition>,
    
    #[serde(default)]
    pub traits: HashMap<String, TraitDefinition>,
}

impl Default for Schema {
    fn default() -> Self {
        let mut types = HashMap::new();
        // Built-in 'page' type as fallback
        types.insert("page".to_string(), TypeDefinition::default());
        
        Schema {
            types,
            traits: HashMap::new(),
        }
    }
}

/// Definition of a type (person, meeting, project, etc.)
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct TypeDefinition {
    #[serde(default)]
    pub fields: HashMap<String, FieldDefinition>,
    
    #[serde(default)]
    pub detect: Option<DetectionRules>,
}

/// Definition of a trait (@task, @remind, @highlight, etc.)
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct TraitDefinition {
    #[serde(default)]
    pub fields: HashMap<String, FieldDefinition>,
    
    #[serde(default)]
    pub cli: Option<TraitCliConfig>,
}

/// CLI configuration for a trait
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TraitCliConfig {
    /// Alias command name (e.g., "tasks" creates `rvn tasks`)
    pub alias: Option<String>,
    
    /// Default query filter when using the alias
    pub default_query: Option<String>,
}

/// Definition of a field within a type or trait
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FieldDefinition {
    #[serde(rename = "type")]
    pub field_type: FieldType,
    
    #[serde(default)]
    pub required: bool,
    
    #[serde(default)]
    pub default: Option<serde_json::Value>,
    
    /// For enum types: allowed values
    #[serde(default)]
    pub values: Option<Vec<String>>,
    
    /// For ref types: target type name
    #[serde(default)]
    pub target: Option<String>,
    
    /// For number types: minimum value
    #[serde(default)]
    pub min: Option<f64>,
    
    /// For number types: maximum value
    #[serde(default)]
    pub max: Option<f64>,
    
    /// How to derive the value (e.g., "from_filename")
    #[serde(default)]
    pub derived: Option<String>,
    
    /// For traits: whether this field can be positional
    #[serde(default)]
    pub positional: bool,
}

/// Field types supported by the schema
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "lowercase")]
pub enum FieldType {
    String,
    #[serde(rename = "string[]")]
    StringArray,
    Number,
    #[serde(rename = "number[]")]
    NumberArray,
    Date,
    #[serde(rename = "date[]")]
    DateArray,
    Datetime,
    Enum,
    Bool,
    Ref,
    #[serde(rename = "ref[]")]
    RefArray,
}

/// Detection rules for auto-detecting type from file
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DetectionRules {
    /// Regex pattern to match against file path
    #[serde(default)]
    pub path_pattern: Option<String>,
    
    /// Frontmatter attributes to match
    #[serde(default)]
    pub attribute: Option<HashMap<String, serde_json::Value>>,
}

/// A parsed field value
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(untagged)]
pub enum FieldValue {
    String(String),
    Number(f64),
    Bool(bool),
    Date(String),          // ISO 8601 date string
    Datetime(String),      // ISO 8601 datetime string
    Ref(String),           // Reference target ID
    Array(Vec<FieldValue>),
    Null,
}

impl FieldValue {
    pub fn as_str(&self) -> Option<&str> {
        match self {
            FieldValue::String(s) => Some(s),
            FieldValue::Date(s) => Some(s),
            FieldValue::Datetime(s) => Some(s),
            FieldValue::Ref(s) => Some(s),
            _ => None,
        }
    }
    
    pub fn as_f64(&self) -> Option<f64> {
        match self {
            FieldValue::Number(n) => Some(*n),
            _ => None,
        }
    }
    
    pub fn as_bool(&self) -> Option<bool> {
        match self {
            FieldValue::Bool(b) => Some(*b),
            _ => None,
        }
    }
}

impl From<&str> for FieldValue {
    fn from(s: &str) -> Self {
        FieldValue::String(s.to_string())
    }
}

impl From<String> for FieldValue {
    fn from(s: String) -> Self {
        FieldValue::String(s)
    }
}

impl From<f64> for FieldValue {
    fn from(n: f64) -> Self {
        FieldValue::Number(n)
    }
}

impl From<bool> for FieldValue {
    fn from(b: bool) -> Self {
        FieldValue::Bool(b)
    }
}
