use aws_sdk_s3::{Client, Error};
use rusty_s3::{Bucket, Credentials, S3Action, UrlStyle};

use std::env;
use std::time::Duration;

#[tokio::main]
async fn main() -> Result<(), Error> {
    let shared_config = aws_config::load_from_env().await;
    let client = Client::new(&shared_config);

    let req = client.list_buckets();
    let resp = req.send().await?;
    let buckets = resp.buckets().unwrap_or_default();
    let num_buckets = buckets.len();

    for bucket in buckets {
        println!("{}", bucket.name().unwrap_or_default());
    }

    println!();
    println!("Found {} buckets", num_buckets);
    Ok(())
}

fn omain() {
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
    let action = bucket.list_objects_v2(Some(&credentials));
    let url = action.sign(presigned_url_duration);
    println!("GET {}", url);

    let http_client = reqwest::blocking::Client::new();
    let res = http_client.get(url).send().unwrap().text().unwrap();
    println!("res: {}", res);

    let list = rusty_s3::actions::ListObjectsV2::parse_response(&res).unwrap();
    list.contents
        .iter()
        .for_each(|contents| println!("  {} {}", contents.last_modified, contents.key));
}
