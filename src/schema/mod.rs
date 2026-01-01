//! Schema module - handles loading and validating schema.yaml

mod types;
pub mod loader;
mod validator;

pub use types::*;
pub use loader::{load_schema, create_default_schema};
pub use validator::validate_fields;
