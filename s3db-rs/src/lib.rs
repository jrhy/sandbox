extern crate serde;
use serde::Deserialize;

use std::os::raw::c_char;

extern "C" {
    pub fn ReadRoot(gob: *const u8, goblen: usize) -> *const c_char;
}

#[derive(Debug, Deserialize)]
pub struct MastRoot {
    #[serde(rename = "Link")]
    pub link: Option<String>,
    #[serde(rename = "Size")]
    pub size: u64,
    #[serde(rename = "Height")]
    pub height: u8,
    #[serde(rename = "BranchFactor")]
    pub branch_factor: u32,
    #[serde(rename = "NodeFormat")]
    pub node_format: String,
}

#[derive(Debug, Deserialize)]
pub struct Root {
    #[serde(rename = "Root")]
    pub mast: MastRoot,
    #[serde(rename = "Created")]
    pub created: Option<String>,
    #[serde(rename = "Source")]
    pub source: String,
    #[serde(rename = "MergeSources")]
    pub merge_sources: std::collections::LinkedList<String>,
    #[serde(rename = "MergeMode")]
    pub merge_mode: Option<u8>,
}
