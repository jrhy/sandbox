use rusoto_core::Region;
use rusoto_s3::{ListObjectsV2Request, S3Client, S3};

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
        endpoint: endpoint,
    };
    let client = S3Client::new(custom_region);
    let mut req: ListObjectsV2Request = Default::default();
    req.bucket = "s3db-rs".to_owned();
    req.prefix = Some("/root/current".to_owned());
    match client.list_objects_v2(req).await {
        Ok(output) => {
            println!("roots:");
            output.contents.unwrap().iter().for_each(|o| {
                println!(
                    "  {} {}",
                    o.key.as_ref().unwrap(),
                    o.last_modified.as_ref().unwrap()
                )
            });
        }
        Err(error) => {
            println!("Error: {:?}", error);
        }
    };
}
