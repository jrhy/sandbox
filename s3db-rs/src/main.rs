extern crate gob;
#[macro_use]
// extern crate serde_derive;
extern crate serde;
use gob::Deserializer;
use serde::Deserialize;
//use gob::{error::ErrorKind, StreamDeserializer};
// use std::option::Option;

fn main() {
    println!("Hello, world!");
}

#[derive(Debug, Deserialize)]
pub struct MastRoot {
    #[serde(rename = "Link")]
    Link: Option<String>,
    #[serde(rename = "Link")]
    Size: u64,
    #[serde(rename = "Size")]
    Height: u8,
    #[serde(rename = "Height")]
    BranchFactor: u32,
    #[serde(rename = "NodeFormat")]
    NodeFormat: String,
}

#[derive(Debug, Deserialize)]
pub struct Root {
    #[serde(rename = "Root")]
    Root: MastRoot,
    #[serde(rename = "Created")]
    Created: Option<String>,
    #[serde(rename = "MergeSources")]
    MergeSources: std::collections::LinkedList<String>,
    #[serde(rename = "MergeNode")]
    MergeMode: bool,
}

#[test]
fn goo() {
    let buffer = include_bytes!("../1MOlCL_1nv7fJlQpPVyQnxU");

    let deserializer = Deserializer::from_slice(buffer);
    let decoded = Root::deserialize(deserializer).unwrap();
    println!("{:?}", decoded);
    //assert_eq!(decoded, &[]);
}

#[test]
fn point_struct() {
    #[derive(Deserialize)]
    struct Point {
        #[serde(rename = "X")]
        x: i64,
        #[serde(rename = "Y")]
        y: i64,
    }

    let buffer = include_bytes!("../1MOlCL_1nv7fJlQpPVyQnxU");
    let deserializer = Deserializer::from_slice(buffer);

    let decoded = Point::deserialize(deserializer).unwrap();
    assert_eq!(decoded.x, 22);
    assert_eq!(decoded.y, 33);
}
