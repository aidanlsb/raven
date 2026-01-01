//! Parser module - handles parsing markdown files

mod frontmatter;
mod markdown;
pub mod type_decl;
mod traits;
mod refs;
mod document;

pub use document::{ParsedDocument, ParsedObject, ParsedTrait, ParsedRef, parse_document};
