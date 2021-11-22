extern crate gob;
#[macro_use]
extern crate serde_derive;
extern crate serde;
use serde::Deserialize;
use gob::{Deserializer};
//use gob::{error::ErrorKind, StreamDeserializer};
use std::option::Option;

fn main() {
    println!("Hello, world!");
}

#[derive(Debug,Deserialize)]
struct MastRoot {
          Link         Option<String>
          Size         u64
          Height       u8      
          BranchFactor uint
          NodeFormat   String
}
#[derive(Debug,Deserialize)]
struct Root {
    MastRoot MastRoot
          Created      Option<String>
          MergeSources List<String>
          MergeMode    int        
}

#[test]
fn goo() {
    let buffer = include_bytes!("../1MOlCL_1nv7fJlQpPVyQnxU");

    let deserializer = Deserializer::from_slice(buffer);
    let decoded = Root::deserialize(deserializer).unwrap();
    println!("{:?}", decoded);
    //assert_eq!(decoded, &[]);
}


