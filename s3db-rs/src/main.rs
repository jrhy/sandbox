use bytes::Bytes;
use rusty_s3::{Bucket, Credentials, S3Action, UrlStyle};
use std::env;
use std::io::Write;
use std::string::ToString;
use std::time::Duration;

use std::collections::HashSet;

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
        #[structopt(parse(try_from_str = duration_str::parse), long = "older-than")]
        duration: Duration,
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

struct S3Cli {
    bucket: Bucket,
    duration: Duration,
    credentials: Credentials,
    http_client: reqwest::blocking::Client,
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
    let bucket = Bucket::new(endpoint, path_style, (&args.bucket).to_owned(), region)
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

    let s3 = S3Cli {
        bucket,
        credentials,
        duration: Duration::from_secs(60),
        http_client: reqwest::blocking::Client::new(),
    };
    match args.cmd {
        Command::Vacuum { duration } => vacuum(&s3, duration, &args.prefix, &decryption_key)?,

        Command::Backup {
            ref scope,
            ref dest,
        } => {
            let prefix = match scope {
                BackupScope::Current => format!(
                    "{}/root/current",
                    args.prefix.as_ref().unwrap_or(&"".to_owned())
                ),
                BackupScope::All => {
                    format!("{}/root", args.prefix.as_ref().unwrap_or(&"".to_owned()))
                }
            };

            foreach_object(&s3, Some(&prefix), |c| {
                // TODO: embrce PathBuf
                //println!("first root: {}", root_key);
                let bytes = get_object(&s3, &c.key)?;
                let root = s3db::read_root(&bytes).chain_err(|| "read_root")?;
                //println!("read root: {:?}", root);
                let node_prefix = &args
                    .prefix
                    .as_ref()
                    .map(|p| format!("{}/node/", p))
                    .unwrap_or_else(|| "node/".to_owned());

                dump_tree(
                    &s3,
                    &root.mast.link.unwrap_or_else(|| "".to_owned()),
                    node_prefix,
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
                    c.key
                );
                println!("writing to {}", &root_path);
                let mut f = std::fs::File::create(&root_path)
                    .chain_err(|| format!("creating local root {}", &root_path))?;
                f.write(&bytes).chain_err(|| "writing root")?;
                Ok(())
            })?;
        }
    }
    Ok(())
}

fn vacuum(
    s3: &S3Cli,
    duration: Duration,
    prefix: &Option<String>,
    decryption_key: &Option<Bytes>,
) -> Result<()> {
    // use duration_str::parse;
    use std::time::SystemTime;
    // let cutoff = match parse(duration) {
    // Ok(duration) => SystemTime::now().checked_sub(duration).unwrap(),
    // Err(ref err) => bail!("{}", err),
    // };
    use chrono::{DateTime, Utc};
    let cutoff: DateTime<Utc> = SystemTime::now().checked_sub(duration).unwrap().into();
    println!("cutoff time is {}", cutoff);
    // TODO: this normalization should be done for args.prefix instead.
    let prefix = match prefix {
        Some(prefix) => prefix.to_owned(),
        None => "".to_owned(),
    };
    let prefix = prefix.strip_prefix('/').unwrap_or(&prefix);
    let prefix = prefix.strip_suffix('/').unwrap_or(prefix);
    let prefix = if !prefix.is_empty() {
        prefix.to_owned() + "/"
    } else {
        "".to_owned()
    };
    let current_root_prefix = prefix.clone() + "root/current/";
    let merged_root_prefix = prefix.clone() + "root/merged/";
    let node_prefix = prefix + "node/";
    let mut preserve_roots = HashSet::<String>::new();
    let mut preserve_nodes = HashSet::<String>::new();
    let mut deletable_roots = HashSet::<String>::new();
    let mut deletable_nodes = HashSet::<String>::new();

    // preserve all the nodes in all the current versions
    println!("scanning {}...", current_root_prefix);
    foreach_object(s3, Some(&current_root_prefix), |c| {
        add_new_nodes_from_root(
            s3,
            &c.key,
            &current_root_prefix,
            &node_prefix,
            decryption_key,
            &mut preserve_nodes,
            &mut preserve_roots,
        )
    })?;

    println!("scanning {}...", merged_root_prefix);
    foreach_object(s3, Some(&merged_root_prefix), |c| {
        //println!("  {:?}", c);
        let root_name = match c.key.strip_prefix(&merged_root_prefix) {
                    None => panic!("unexpected key {}, for prefix {}, maybe S3 doesn't work the way you think it does?", c.key, merged_root_prefix),
                    Some(rest) => rest,
                };
        if preserve_roots.contains(root_name) {
            println!("skipping root {} in transition", root_name);
            return Ok(());
        }
        let lastmod = chrono::DateTime::parse_from_rfc3339(&c.last_modified)
            .chain_err(|| format!("parse timestamp of {}", c.key))?;
        if lastmod >= cutoff {
            println!("skipping too-young root {}", root_name);
            add_new_nodes_from_root(
                s3,
                &c.key,
                &merged_root_prefix,
                &node_prefix,
                decryption_key,
                &mut preserve_nodes,
                &mut preserve_roots,
            )?;
            return Ok(());
        }
        println!("root {} looks deletable", root_name);
        deletable_roots.insert(c.key.to_owned());
        Ok(())
    })?;
    println!("scanning {}...", node_prefix);
    foreach_object(s3, Some(&node_prefix), |c| {
        //println!("  {:?}", c);
        let node_name = match c.key.strip_prefix(&node_prefix) {
                    None => panic!("unexpected key {}, for prefix {}, maybe S3 doesn't work the way you think it does?", c.key, node_prefix),
                    Some(rest) => rest,
                };
        if preserve_nodes.contains(node_name) {
            println!("skipping current node {}", node_name);
            return Ok(());
        }
        let lastmod = chrono::DateTime::parse_from_rfc3339(&c.last_modified)
            .chain_err(|| "parse timestamp")?;
        if lastmod < cutoff {
            println!("node {} looks deletable", node_name);
            deletable_nodes.insert(c.key.to_owned());
        } else {
            println!("skipping too-young node {}", node_name);
        }
        Ok(())
    })?;
    println!("summary:");
    for root in preserve_roots.iter() {
        println!("roots to preserve: {}", root)
    }
    for node in preserve_nodes.iter() {
        println!("node to preserve: {}", node)
    }
    for root in deletable_roots.iter() {
        println!("deletable root: {}", root)
    }
    for node in deletable_nodes.iter() {
        println!("deletable node: {}", node)
    }

    Ok(())
}

fn add_new_nodes_from_root(
    s3: &S3Cli,
    link: &str,
    root_prefix: &str,
    node_prefix: &str,
    decryption_key: &Option<Bytes>,
    nodes: &mut HashSet<String>,
    roots: &mut HashSet<String>,
) -> Result<()> {
    let root_name = match link.strip_prefix(root_prefix) {
        None => panic!(
            "unexpected key {}, for prefix {}, maybe S3 doesn't work the way you think it does?",
            link, root_prefix
        ),
        Some(rest) => rest,
    };
    let root_bytes = get_object(s3, link)?;
    let root = s3db::read_root(&root_bytes).chain_err(|| "read_root")?;
    root.mast
        .link
        .as_ref()
        .map(|link| {
            visit_tree(s3, link, node_prefix, decryption_key, |e| match e {
                VisitTree::ShouldVisit(link) => {
                    let already_seen = nodes.contains(link);
                    if already_seen {
                        println!(" already visited {}", link);
                    }
                    Ok(!already_seen)
                }
                VisitTree::DoneVisit(link, _, _) => {
                    println!(" preserving {}", link);
                    Ok(nodes.insert(link))
                }
            })
        })
        .unwrap_or(Ok(()))
        .chain_err(|| "visit tree")?;
    roots.insert(root_name.to_owned());
    Ok(())
}

fn foreach_object<F>(s3: &S3Cli, prefix: Option<&str>, mut cb: F) -> Result<()>
where
    F: FnMut(&rusty_s3::actions::list_objects_v2::ListObjectsContent) -> Result<()>,
{
    let mut next: Option<String> = None;
    loop {
        let mut action = s3.bucket.list_objects_v2(Some(&s3.credentials));
        if let Some(ref next) = next {
            action.query_mut().insert("continuation-token", next);
        };
        if let Some(ref prefix) = prefix {
            action.query_mut().insert("prefix", prefix.to_owned());
        };
        let url = action.sign(s3.duration);

        let res = s3
            .http_client
            .get(url)
            .send()
            .chain_err(|| "HTTP GET")?
            .text()
            .chain_err(|| "parse S3 list")?;

        let list = rusty_s3::actions::ListObjectsV2::parse_response(&res)
            .chain_err(|| format!("parse ListObjectsV2 response {}", res))?;
        for c in list.contents.iter() {
            if let Err(e) = cb(c) {
                return Err(e);
            }
        }
        if list.next_continuation_token.is_none() {
            return Ok(());
        }
        next = list.next_continuation_token
    }
}

fn get_object(s3: &S3Cli, key: &str) -> Result<Bytes> {
    let url = s3
        .bucket
        .get_object(Some(&s3.credentials), key)
        .sign(s3.duration);
    let response = s3.http_client.get(url).send().chain_err(|| "GET")?;

    if !response.status().is_success() {
        return Err(format!(
            "failed loading root {}: received HTTP {} from S3",
            key,
            response.status()
        )
        .into());
    }
    response.bytes().chain_err(|| "to bytes")
}

fn dump_tree(
    s3: &S3Cli,
    key: &str, // XXX this is actually a link name, not an S3 key
    node_prefix: &str,
    decryption_key: &Option<Bytes>,
    output_dir: &str,
) -> Result<()> {
    visit_tree_caching(
        s3,
        key,
        node_prefix,
        decryption_key,
        output_dir,
        |e| match e {
            VisitTreeCaching::ShouldVisit(_, maybe_cache) => {
                if maybe_cache.is_none() {
                    println!("skipping already-cached node {}\n", key);
                    Ok(true)
                } else {
                    Ok(false)
                }
            }
            VisitTreeCaching::DoneVisit(_, maybe_cache) => match maybe_cache {
                Some((bytes, mut file)) => {
                    // We only write a node's bytes if its subtree has been successfully
                    // backed up, so if we come back for another version, we can skip it.
                    file.write(&bytes)
                        .chain_err(|| format!("write node bytes for {}", key))?;
                    Ok(true)
                }
                None => Ok(true),
            },
        },
    )
}

enum VisitTreeCaching<'a> {
    ShouldVisit(&'a s3db::Node, &'a Option<(Bytes, std::fs::File)>),
    DoneVisit(&'a s3db::Node, Option<(Bytes, std::fs::File)>),
}

fn visit_tree_caching<F>(
    s3: &S3Cli,
    key: &str, // XXX this is actually a link name, not an S3 key
    node_prefix: &str,
    decryption_key: &Option<Bytes>,
    output_dir: &str,
    mut cb: F,
) -> Result<()>
where
    F: FnMut(VisitTreeCaching) -> Result<bool>,
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
            Some(Cur::Done(node, maybe_cache)) => {
                cb(VisitTreeCaching::DoneVisit(&node, maybe_cache))?;
            }
            Some(Cur::Todo { ref link }) => {
                let (node, maybe_cache) = get_node(
                    &s3.bucket,
                    &s3.credentials,
                    &s3.http_client,
                    s3.duration,
                    node_prefix,
                    link,
                    decryption_key,
                    output_dir,
                )
                .chain_err(|| "get_node")?;
                if !cb(VisitTreeCaching::ShouldVisit(&node, &maybe_cache))? {
                    continue;
                }
                stack.push(Cur::Done(node.clone(), maybe_cache));
                for l in (&node.links).iter().rev() {
                    if !l.is_empty() {
                        stack.push(Cur::Todo { link: l.to_owned() })
                    }
                }
            }
        }
    }
    Ok(())
}

enum VisitTree<'a> {
    ShouldVisit(&'a str),
    DoneVisit(String, s3db::Node, Bytes),
}

fn visit_tree<F>(
    s3: &S3Cli,
    link: &str,
    node_prefix: &str,
    decryption_key: &Option<Bytes>,
    mut cb: F,
) -> Result<()>
where
    F: FnMut(VisitTree) -> Result<bool>,
{
    enum Cur {
        Todo { link: String },
        Done(String, s3db::Node, Bytes),
    }
    let mut stack = vec![Cur::Todo {
        link: link.to_owned(),
    }];
    loop {
        match stack.pop() {
            None => break,
            Some(Cur::Done(link, node, bytes)) => {
                cb(VisitTree::DoneVisit(link, node, bytes))?;
            }
            Some(Cur::Todo { link }) => {
                if !cb(VisitTree::ShouldVisit(&link))? {
                    println!("shouldn't visit link!");
                    continue;
                }
                let key = node_prefix.to_owned() + &link;
                let bytes = get_object(s3, &key)?;
                let node = s3db::read_node(&bytes, decryption_key)
                    .chain_err(|| format!("read_node from {}", &key))?;

                stack.push(Cur::Done(link, node.clone(), bytes));
                for l in (&node.links).iter().rev() {
                    if !l.is_empty() {
                        stack.push(Cur::Todo { link: l.to_owned() })
                    }
                }
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
