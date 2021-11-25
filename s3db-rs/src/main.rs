use rusoto_core::Region;
use rusoto_s3::{S3Client, S3};

use std::env;

#[tokio::main]
async fn main() {
    let region = env::var("AWS_REGION").expect("AWS_REGION is set and a valid String");
    let endpoint = env::var("S3_ENDPOINT")
        .expect("S3_ENDPOINT is set and a valid String")
        .parse()
        .expect("endpoint is a valid Url");
    let custom_region = Region::Custom {
        name: region,
        endpoint,
    };
    let client = S3Client::new(custom_region);
    let req = rusoto_s3::ListObjectsV2Request {
        bucket: "s3db-rs".to_owned(),
        prefix: Some("/root/current".to_owned()),
        ..Default::default()
    };
    match client.list_objects_v2(req).await {
        Ok(output) => {
            println!("roots:");
            if let Some(ref contents) = output.contents {
                contents.iter().for_each(|o| {
                    if let (Some(k), Some(l)) = (&o.key, &o.last_modified) {
                        println!("  {} {}", k, l)
                    }
                })
            }
        }
        Err(error) => {
            println!("Error: {:?}", error);
        }
    };
}
