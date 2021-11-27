extern crate bytes;
extern crate serde;
use serde::Deserialize;

use std::ffi::CStr;
use std::os::raw::c_char;

extern "C" {
    fn ReadNode(gob: *const u8, goblen: usize) -> *const c_char;
    fn ReadRoot(gob: *const u8, goblen: usize) -> *const c_char;
}

fn read_json(
    bytes: bytes::Bytes,
    reader: unsafe extern "C" fn(*const u8, usize) -> *const c_char,
) -> Result<String, std::io::Error> {
    let json = unsafe {
        let json_c = reader(bytes.as_ptr(), bytes.len());
        if json_c.is_null() {
            return Result::Err(std::io::Error::new(
                std::io::ErrorKind::InvalidData,
                "unmarshal gob",
            ));
        }
        CStr::from_ptr(json_c).to_str().unwrap()
    };
    println!("got json: {}", json);
    Ok(json.to_owned())
}

pub fn read_node(bytes: bytes::Bytes) -> Result<Root, std::io::Error> {
    Ok(serde_json::from_str(&read_json(bytes, ReadNode)?)?)
}

pub fn read_root(bytes: bytes::Bytes) -> Result<Root, std::io::Error> {
    Ok(serde_json::from_str(&read_json(bytes, ReadRoot)?)?)
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
    pub merge_sources: Option<std::collections::LinkedList<String>>,
    #[serde(rename = "MergeMode")]
    pub merge_mode: Option<u8>,
}
