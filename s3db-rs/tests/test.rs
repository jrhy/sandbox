#[test]
fn read_root() {
    let buffer = include_bytes!("1MOlCL_1nv7fJlQpPVyQnxU");
    let root = s3db::read_root(bytes::Bytes::from_static(buffer)).unwrap();

    //{"MergeSources":["1MOlCL_yNhj9P1f2aGpmgoQ"],"Source":"","Root":{"Link":"lPSxUoQLXRzZAvYQzaBy-rQIBh_lIBRIy47ny41kQt4","Size":5125,"Height":5,"BranchFactor":4,"NodeFormat":"v1.1.5binary"}}

    // let root: s3db::Root = serde_json::from_str(json).unwrap();
    println!("{:?}", root);
    assert_eq!(4, root.mast.branch_factor);
    assert_eq!(5125, root.mast.size);
    assert_eq!(5, root.mast.height);
    assert_eq!("v1.1.5binary", &root.mast.node_format);
    assert_eq!(None, root.created);
    assert_eq!("", &root.source);
    //assert_eq!(Some(["1MOlCL_yNhj9P1f2aGpmgoQ"]), root.merge_sources);
    //assert_eq!(Some("lPSxUoQLXRzZAvYQzaBy-rQIBh_lIBRIy47ny41kQt4"), &root.mast.link);
}
