extern crate bytes;
extern crate serde;
use bytes::Bytes;
use serde::Deserialize;
use std::convert::TryFrom;

use std::ffi::CStr;
use std::os::raw::c_char;

#[macro_use]
extern crate error_chain;

mod errors {
    error_chain! {}
}
use errors::*;

pub mod node_crypt;
mod varint;

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

#[derive(Debug, Deserialize, PartialEq)]
pub struct Node {
    pub links: Vec<String>,
}

extern "C" {
    fn ReadRoot(gob: *const u8, goblen: usize) -> *const c_char;
}

fn read_json(
    bytes: &Bytes,
    reader: unsafe extern "C" fn(*const u8, usize) -> *const c_char,
) -> Result<String> {
    let json = unsafe {
        let json_c = reader(bytes.as_ptr(), bytes.len());
        if json_c.is_null() {
            bail!("unmarshal gob")
        }
        CStr::from_ptr(json_c).to_str().unwrap()
    };
    Ok(json.to_owned())
}

pub fn read_node(bytes: &Bytes, key: &Option<Bytes>) -> Result<Node> {
    let decrypted: Bytes;
    let bytes = match key {
        Some(ref key) => {
            decrypted = Bytes::from(node_crypt::decrypt(key.as_ref(), bytes)?);
            &decrypted
        }
        None => bytes,
    };
    //println!("reading {}-byte node", bytes.len());

    let mut c: usize = 0;
    let (nc, _) = read_v115_array(&bytes.slice(c..)).chain_err(|| "read keys")?;
    c += nc;
    let (nc, _) = read_v115_array(&bytes.slice(c..)).chain_err(|| "read values")?;
    c += nc;

    let (_, links_bytes) = read_v115_array(&bytes.slice(c..)).chain_err(|| "read links")?;
    let mut links = Vec::<String>::new();
    for l in links_bytes.iter() {
        let s = String::from_utf8(Vec::from(l.as_ref())).chain_err(|| "link")?;
        links.push(s);
    }
    Ok(Node { links })
}

fn read_v115_array(bytes: &Bytes) -> Result<(usize, Vec<Bytes>)> {
    let mut c: usize = 0;
    let (mut entry, i): (u64, i32) = varint::read(bytes);
    if i < 0 {
        bail!("truncated before entries")
    };
    let i: usize = usize::try_from(i).chain_err(|| "way too many entries")?;
    c += i;
    let mut res = Vec::<Bytes>::new();
    let mut entries_read = 0;
    while entry > 0 {
        let (key_bytes_u64, i) = varint::read(&bytes.slice(c..));
        let key_bytes = usize::try_from(key_bytes_u64).chain_err(|| "too many")?;
        if i < 0 {
            bail!("node truncated in entry length")
        }
        let i: usize = usize::try_from(i).chain_err(|| "way too many")?;
        if c + i + key_bytes > bytes.len() {
            bail!("node truncated in entries after {}", entries_read)
        }
        res.push(bytes.slice(c + i..c + i + key_bytes));
        c += i + key_bytes;
        entry -= 1;
        entries_read += 1;
    }
    Ok((c, res))
}

pub fn read_root(bytes: &Bytes) -> Result<Root> {
    serde_json::from_str(&read_json(bytes, ReadRoot).chain_err(|| "read")?).chain_err(|| "json")
}
