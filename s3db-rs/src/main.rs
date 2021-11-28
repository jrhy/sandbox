use rusty_s3::{Bucket, Credentials, S3Action, UrlStyle};
use std::env;
use std::time::Duration;

fn main() {
    // setting up a bucket
    let endpoint = env::var("S3_ENDPOINT")
        .expect("S3_ENDPOINT is set and a valid String")
        .parse()
        .expect("endpoint is a valid Url");
    let path_style = UrlStyle::Path;
    let name = "s3db-rs";
    let region = env::var("AWS_REGION").expect("AWS_REGION is set and a valid String");
    let bucket =
        Bucket::new(endpoint, path_style, name, region).expect("Url has a valid scheme and host");

    // setting up the credentials
    let key = env::var("AWS_ACCESS_KEY_ID").expect("AWS_ACCESS_KEY_ID is set and a valid String");
    let secret =
        env::var("AWS_SECRET_ACCESS_KEY").expect("AWS_ACCESS_KEY_ID is set and a valid String");
    let credentials = Credentials::new(key, secret);

    let presigned_url_duration = Duration::from_secs(60);
    let mut action = bucket.list_objects_v2(Some(&credentials));
    action.query_mut().insert("prefix", "root/current");
    let url = action.sign(presigned_url_duration);
    println!("GET {}", url);

    let http_client = reqwest::blocking::Client::new();
    let res = http_client.get(url).send().unwrap().text().unwrap();
    println!("res: {}", res);

    let list = rusty_s3::actions::ListObjectsV2::parse_response(&res).unwrap();
    if false {
        list.contents
            .iter()
            .for_each(|contents| println!("  {} {}", contents.last_modified, contents.key));
    } else {
        let first = list.contents.first().map(|c| &c.key);
        match first {
            Some(key) => {
                println!("first root: {}", key);
                let url = bucket
                    .get_object(Some(&credentials), key)
                    .sign(presigned_url_duration);
                let bytes = http_client.get(url).send().unwrap().bytes().unwrap();
                let root = s3db::read_root(&bytes).unwrap();
                println!("read root: {:?}", root);

                fn dump_tree(
                    key: &str,
                    bucket: &Bucket,
                    credentials: &Credentials,
                    http_client: &reqwest::blocking::Client,
                    duration: Duration,
                ) {
                    let url = bucket
                        .get_object(Some(credentials), &format!("node/{}", key))
                        .sign(duration);
                    let bytes = http_client.get(url).send().unwrap().bytes().unwrap();
                    let node = s3db::read_node(&bytes, None).unwrap();
                    println!("read node: {:?}", node);
                    for ref l in node.links {
                        if !l.is_empty() {
                            dump_tree(l, bucket, credentials, http_client, duration)
                        }
                    }
                }
                dump_tree(
                    &root.mast.link.unwrap(),
                    &bucket,
                    &credentials,
                    &http_client,
                    presigned_url_duration,
                );
            }
            None => println!("no roots to show"),
        }
    };
}
