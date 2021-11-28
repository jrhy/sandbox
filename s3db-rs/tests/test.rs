#[test]
fn read_root() {
    let buffer = include_bytes!("1MOlCL_1nv7fJlQpPVyQnxU");
    let root = s3db::read_root(&bytes::Bytes::from_static(buffer)).unwrap();

    //{"MergeSources":["1MOlCL_yNhj9P1f2aGpmgoQ"],"Source":"","Root":{"Link":"lPSxUoQLXRzZAvYQzaBy-rQIBh_lIBRIy47ny41kQt4","Size":5125,"Height":5,"BranchFactor":4,"NodeFormat":"v1.1.5binary"}}

    // let root: s3db::Root = serde_json::from_str(json).unwrap();
    println!("{:?}", root);
    assert_eq!(4096, root.mast.branch_factor);
    assert_eq!(5001, root.mast.size);
    assert_eq!(1, root.mast.height);
    assert_eq!("v1.1.5binary", &root.mast.node_format);
    assert_eq!(None, root.created);
    assert_eq!("", &root.source);
    //assert_eq!(Some(["1MOlCL_yNhj9P1f2aGpmgoQ"]), root.merge_sources);
    //assert_eq!(Some("lPSxUoQLXRzZAvYQzaBy-rQIBh_lIBRIy47ny41kQt4"), &root.mast.link);
}

#[test]
fn read_node() {
    let buffer = include_bytes!("YKPRjxKOEttHaAN97ccSrsIT6-CFNj9znrP-4x8-dhE");
    let node = s3db::read_node(&bytes::Bytes::from_static(buffer), None).unwrap();
    println!("node: {:?}", node);
    assert_eq!(
        vec!(
            "WQEhPEvtWvhrkQsik-aSctOVbPOVmHyGyPRVUyFcBU4",
            "oWdeIY5t4XBGXfzPAVwJxfNwk_aHLWYYN-sdZlTZhto"
        ),
        node.links
    );
}

#[test]
fn test_decrypt() {
    let buffer = include_bytes!("PuLGxFL_mnxYtv6CkyWkBnbE5LRsCY1ZuIv9MQoKvgU");
    let key = s3db::node_crypt::derive_key(b"keykeykey\n", &[]);
    let bytes = bytes::Bytes::from(buffer.as_ref());
    let node = s3db::read_node(&bytes, Some(&key)).unwrap();
    println!("node: {:?}", node);
    assert_eq!(s3db::Node{links: Vec::new()}, node);
}
