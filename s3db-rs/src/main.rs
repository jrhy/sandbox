use bytes::Bytes;
use rusty_s3::{Bucket, Credentials, S3Action, UrlStyle};
use std::env;
use std::io::Write;
use std::string::ToString;
use std::time::Duration;

use structopt::StructOpt;

#[macro_use]
extern crate error_chain;

// We'll put our errors in an `errors` module, and other modules in
// this crate will `use errors::*;` to get access to everything
// `error_chain!` creates.
mod errors {
    // Create the Error, ErrorKind, ResultExt, and Result types
    error_chain! {
        foreign_links {
             Io(::std::io::Error) #[cfg(unix)];
        }
    }
}

// This only gives access within this module. Make this `pub use errors::*;`
// instead if the types must be accessible from other modules (e.g., within
// a `links` section).
use errors::*;

/// Ensure S3_ENDPOINT, AWS_REGION, AWS_ACCESS_KEY_ID, and AWS_SECRET_ACCESS_KEY are set.
#[derive(StructOpt)]
struct Cli {
    /// name of S3 bucket to use
    #[structopt(short = "b")]
    bucket: String,
    /// prefix of the DB (in bucket)
    #[structopt(short = "p")]
    prefix: Option<String>,
    /// decryption key file path
    #[structopt(short = "k", parse(from_os_str))]
    key: Option<std::path::PathBuf>,
    #[structopt(subcommand)]
    cmd: Command,
}

#[derive(StructOpt)]
enum Command {
    Vacuum {
        #[structopt(long = "before", default_value = "1h")]
        before: String,
    },
    /// Backup the current or all versions
    Backup {
        /// Which versions to include: current (default), all
        #[structopt(long = "scope", default_value = "current")]
        scope: BackupScope,
        /// destination path
        #[structopt(short = "d", parse(from_os_str))]
        // TODO, just say "." is the default, get rid of Option
        dest: Option<std::path::PathBuf>,
    },
}

enum BackupScope {
    Current,
    All,
}
impl std::str::FromStr for BackupScope {
    type Err = String;
    fn from_str(s: &str) -> std::result::Result<Self, String> {
        match s.to_ascii_lowercase().as_ref() {
            "current" => Ok(BackupScope::Current),
            "all" => Ok(BackupScope::All),
            _ => Err("I understand --scope \"all\" or \"current\"".to_owned()),
        }
    }
}

quick_main!(run);

fn run() -> Result<()> {
    let args = Cli::from_args();

    // setting up a bucket
    let endpoint = env::var("S3_ENDPOINT")
        .chain_err(|| "S3_ENDPOINT must be set")?
        .parse()
        .chain_err(|| "S3_ENDPOINT endpoint must be a valid URL")?;
    let path_style = UrlStyle::Path;
    let region = env::var("AWS_REGION").chain_err(|| "AWS_REGION must be set")?;
    let bucket = Bucket::new(endpoint, path_style, args.bucket.to_owned(), region)
        .chain_err(|| "URL has a valid scheme and host")?;

    // setting up the credentials
    let key = env::var("AWS_ACCESS_KEY_ID").chain_err(|| "AWS_ACCESS_KEY_ID must be set")?;
    let secret = env::var("AWS_SECRET_ACCESS_KEY").chain_err(|| "AWS_ACCESS_KEY_ID must be set")?;
    let credentials = Credentials::new(key, secret);

    let decryption_key: Option<Bytes> = match args.key {
        Some(ref path) => {
            let bytes = Bytes::from(std::fs::read(path).chain_err(|| "read_key")?);
            Some(Bytes::from(
                s3db::node_crypt::derive_key(&bytes, &[]).chain_err(|| "derive_key")?,
            ))
        }
        None => None,
    };

    let presigned_url_duration = Duration::from_secs(60);
    let http_client = reqwest::blocking::Client::new();
    let (prefix, dest) = match args.cmd {
        Command::Vacuum { before: _ } => unimplemented!("vacuum"),
        Command::Backup {
            ref dest,
            scope: BackupScope::Current,
        } => (
            format!(
                "{}/root/current",
                args.prefix.as_ref().unwrap_or(&"".to_owned())
            ),
            dest,
        ),
        Command::Backup {
            ref dest,
            scope: BackupScope::All,
        } => (
            format!("{}/root", args.prefix.as_ref().unwrap_or(&"".to_owned())),
            dest,
        ),
    };

    foreach_s3_object(
        &bucket,
        presigned_url_duration,
        &credentials,
        &http_client,
        Some(&prefix),
        |key| {
            // TODO: embrce PathBuf
            //println!("first root: {}", root_key);
            let root_key = key;
            let url = bucket
                .get_object(Some(&credentials), root_key)
                .sign(presigned_url_duration);
            let response = http_client.get(url).send().unwrap();

            if !response.status().is_success() {
                return Err(format!(
                    "failed loading root {}: received HTTP {} from S3",
                    root_key,
                    response.status()
                )
                .into());
            }
            let bytes = response.bytes().unwrap();
            let root = s3db::read_root(&bytes).unwrap();
            //println!("read root: {:?}", root);
            let node_prefix = &args
                .prefix
                .as_ref()
                .map(|p| format!("{}/node/", p))
                .unwrap_or_else(|| "node/".to_owned());

            dump_tree(
                &root.mast.link.unwrap_or_else(|| "".to_owned()),
                &bucket,
                &credentials,
                &http_client,
                presigned_url_duration,
                &node_prefix,
                &decryption_key,
                dest.as_ref()
                    .map(|x| x.to_str().unwrap())
                    .unwrap_or(&".".to_owned()),
            )?;
            let root_path = format!(
                "{}/{}",
                dest.as_ref()
                    .map(|x| x.to_str().unwrap())
                    .unwrap_or(&".".to_owned()),
                root_key
            );
            println!("writing to {}", &root_path);
            let mut f = std::fs::File::create(&root_path)
                .chain_err(|| format!("creating local root {}", &root_path))?;
            f.write(&bytes).chain_err(|| "writing root")?;
            Ok(())
        },
    )?;
    Ok(())
}

fn foreach_s3_object<F>(
    bucket: &Bucket,
    duration: Duration,
    credentials: &Credentials,
    http_client: &reqwest::blocking::Client,
    prefix: Option<&str>,
    mut cb: F,
) -> Result<()>
where
    F: FnMut(&str) -> Result<()>,
{
    let mut next: Option<String> = None;
    loop {
        let mut action = bucket.list_objects_v2(Some(&credentials));
        if let Some(ref next) = next {
            action.query_mut().insert("continuation-token", next);
        };
        if let Some(ref prefix) = prefix {
            action.query_mut().insert("prefix", prefix.to_owned());
        };
        let url = action.sign(duration);

        let res = http_client
            .get(url)
            .send()
            .chain_err(|| "HTTP GET")?
            .text()
            .chain_err(|| "parse S3 list")?;
        //println!("res: {}", res);

        let list = rusty_s3::actions::ListObjectsV2::parse_response(&res)
            .chain_err(|| "parse ListObjectsV2 response")?;
        for c in list.contents.iter() {
            if let Err(e) = cb(&c.key) {
                return Err(e);
            }
        }
        if list.next_continuation_token.is_none() {
            return Ok(());
        }
        next = list.next_continuation_token
    }
}

fn dump_tree(
    key: &str, // XXX this is actually a link name, not an S3 key
    bucket: &Bucket,
    credentials: &Credentials,
    http_client: &reqwest::blocking::Client,
    duration: Duration,
    node_prefix: &str,
    decryption_key: &Option<Bytes>,
    output_dir: &str,
) -> Result<()> {
    visit_tree(
        key,
        bucket,
        credentials,
        http_client,
        duration,
        node_prefix,
        decryption_key,
        output_dir,
        |_, maybe_cache| {
            if maybe_cache.is_none() {
                println!("skipping already-cached node {}\n", key);
                Ok(true)
            } else {
                Ok(false)
            }
        },
        |_, maybe_cache| {
            match maybe_cache {
                Some((bytes, mut file)) => {
                    // We only write a node's bytes if its subtree has been successfully
                    // backed up, so if we come back for another version, we can skip it.
                    file.write(&bytes)
                        .chain_err(|| format!("write node bytes for {}", key))?;
                    Ok(())
                }
                None => Ok(()),
            }
        },
    )
}

fn visit_tree<F, G>(
    key: &str, // XXX this is actually a link name, not an S3 key
    bucket: &Bucket,
    credentials: &Credentials,
    http_client: &reqwest::blocking::Client,
    duration: Duration,
    node_prefix: &str,
    decryption_key: &Option<Bytes>,
    output_dir: &str,
    mut should_visit: F,
    mut done_visit: G,
) -> Result<()>
where
    F: FnMut(&s3db::Node, &Option<(Bytes, std::fs::File)>) -> Result<bool>,
    G: FnMut(&s3db::Node, Option<(Bytes, std::fs::File)>) -> Result<()>,
{
    enum Cur {
        Todo { link: String },
        Done(s3db::Node, Option<(Bytes, std::fs::File)>),
    }
    let mut stack = vec![Cur::Todo {
        link: key.to_owned(),
    }];
    loop {
        match stack.pop() {
            None => break,
            Some(Cur::Done(node, maybe_cache)) => done_visit(&node, maybe_cache)?,
            Some(Cur::Todo { ref link }) => {
                let (node, maybe_cache) = get_node(
                    bucket,
                    credentials,
                    http_client,
                    duration,
                    node_prefix,
                    link,
                    decryption_key,
                    output_dir,
                ).chain_err(|| "get_node")?;
                if !should_visit(&node, &maybe_cache)? {
                    continue;
                }
                for l in (&node.links).iter().rev() {
                    if !l.is_empty() {
                        stack.push(Cur::Todo { link: l.to_owned() })
                    }
                }
                stack.push(Cur::Done(node, maybe_cache));
            }
        }
    }
    Ok(())
}

fn get_node(
    bucket: &Bucket,
    credentials: &Credentials,
    http_client: &reqwest::blocking::Client,
    duration: Duration,
    node_prefix: &str,
    key: &str,
    decryption_key: &Option<Bytes>,
    output_dir: &str,
) -> Result<(s3db::Node, Option<(Bytes, std::fs::File)>)> {
    let path = format!("{}{}", node_prefix, key);
    let file_path = format!("{}/{}", output_dir, path);
    let mut file = std::fs::OpenOptions::new()
        .read(true)
        .write(true)
        .create(true)
        .open(&file_path)
        .chain_err(|| format!("open {}", file_path))?;
    let len = file
        .metadata()
        .chain_err(|| format!("{}: get file size", file_path))?
        .len();

    if len != 0 {
        // TODO read_node doesn't need to return bytes
        if let Ok((node, _bytes)) = read_node(&file_path, len, &mut file, decryption_key) {
            return Ok((node, None));
        }
    }

    let url = bucket.get_object(Some(credentials), &path).sign(duration);
    //println!("GET {}", path);
    let response = http_client.get(url).send().unwrap();
    if !response.status().is_success() {
        bail!(
            "failed loading node {}: received HTTP {} from S3",
            key,
            response.status()
        )
    }
    let bytes = response.bytes().chain_err(|| "read HTTP response bytes")?;
    let node = s3db::read_node(&bytes, decryption_key)
        .chain_err(|| format!("read_node from {}", file_path))?;
    Ok((node, Some((bytes, file))))
}

fn read_node(
    file_path: &str,
    len: u64,
    file: &mut std::fs::File,
    decryption_key: &Option<Bytes>,
) -> Result<(s3db::Node, bytes::Bytes)> {
    println!("using cached {}", &file_path);
    use std::convert::TryInto;
    use std::io::Read;
    let mut vec = Vec::<u8>::with_capacity(len.try_into().unwrap());
    file.read_to_end(&mut vec)
        .chain_err(|| format!("{}: read cached bytes", file_path))?;
    let bytes = Bytes::from(vec);
    Ok((
        s3db::read_node(&bytes, decryption_key)
            .chain_err(|| format!("read_node from {}", file_path))?,
        bytes,
    ))
}
