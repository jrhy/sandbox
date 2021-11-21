// use std::borrow::Cow;
use std::cmp::Ordering;
use std::rc::Rc;
use std::result::Result;
use std::vec::Vec;

#[derive(Debug)]
enum MastError {
    InvalidNode,
    StoreError(std::io::Error),
}

#[derive(Debug,Clone)]
struct Node {
    key: Vec<i32>,
    value: Vec<i32>,
    link: Vec<Option<Link>>,
    dirty: bool,
}
/*
// TODO
impl Clone for Node {
    fn clone(&self) -> Node {
        panic!("why are you doing this")
    }
}
impl ToOwned for Node {
    type Owned = Node;
    fn to_owned(&self) -> Self::Owned {
        return *(self.clone());
    }
}*/

#[derive(Clone, Debug)]
enum Link {
    // Empty,
    MutableNode(Node, Option<Rc<Node>>),
    SharedNode(Rc<Node>),
    // Node(Cow<'a, Node<'a>>),
    Stored(String),
}

pub struct Mast<'a> {
    size: u64,
    height: u8,
    root_link: Link,
    branch_factor: u16,
    grow_after_size: u64,
    shrink_below_size: u64,
    key_order: fn(&i32, &i32) -> i8,
    key_layer: fn(&i32, u16) -> u8,
    _a: std::marker::PhantomData<&'a u32>,
    // marshal:
    // unmarshal:
    // store: InMemoryNodeStore<'a>,
}

const default_branch_factor: u16 = 16;

fn default_order(a: &i32, b: &i32) -> i8 {
    if *a < *b {
        return -1;
    } else if *a > *b {
        return 1;
    } else {
        return 0;
    }
}

fn default_layer(v: &i32, branch_factor: u16) -> u8 {
    let mut layer = 0;
    let mut v = *v;
    if branch_factor == 16 {
        while v != 0 && v & 0xf == 0 {
            v >>= 4;
            layer += 1
        }
    } else {
        while v != 0 && v % branch_factor as i32 == 0 {
            v /= branch_factor as i32;
            layer += 1;
        }
    }
    return layer;
}

impl<'a> Mast<'a> {
    pub fn newInMemory() -> Mast<'a> {
        return Mast {
            size: 0,
            height: 0,
            root_link: Link::MutableNode(Node::new(default_branch_factor as usize), None),
            branch_factor: default_branch_factor,
            grow_after_size: default_branch_factor as u64,
            shrink_below_size: 1,
            key_order: default_order,
            key_layer: default_layer,
            _a: std::marker::PhantomData,
            // store: InMemoryNodeStore::new(),
        };
    }

    fn insert(&mut self, key: i32, value: i32) -> Result<InsertResult, MastError> {
        let key_layer = (self.key_layer)(&key, self.branch_factor);
        let target_layer = std::cmp::min(key_layer, self.height);
        let distance = self.height - target_layer;

        let root = load_mut(&mut self.root_link)?;
        let res = root.insert(key, value, distance, self.key_order)?;
        match res {
            InsertResult::Inserted => self.size += 1,
            _ => return Ok(res),
        };

        if self.size > self.grow_after_size
            && root.can_grow(self.height, self.key_layer, self.branch_factor)
        {
            self.root_link = root
                .grow(self.height, self.key_layer, self.branch_factor)
                .unwrap();
            self.height += 1;
            self.shrink_below_size *= self.branch_factor as u64;
            self.grow_after_size *= self.branch_factor as u64;
        };

        Ok(res)
    }

    fn get(&self, key: &i32) -> Result<Option<&i32>, MastError> {
        let mut distance =
            self.height - std::cmp::min((self.key_layer)(key, self.branch_factor), self.height);
        if distance < 0 { panic!("goo") };
        let mut node = load(&self.root_link)?;
        loop {
            let (equal, i) = get_index_for_key(key, &node.key, self.key_order);
            if distance == 0 {
                if equal {
                    return Ok(Some(&node.value[i]));
                } else {
                    return Ok(None);
                }
            } else {
                distance -= 1
            }
            match node.link[i] {
                None => return Ok(None),
                Some(ref link) => node = load(link)?,
            }
        }
    }
}

fn load(link: &Link) -> Result<&Node, MastError> {
    match link {
        // Link::Empty => Ok(Cow::Borrowed(&Node::empty()).to_mut()),
        Link::MutableNode(ref node, _) => Ok(node),
        Link::SharedNode(ref rc) => Ok(rc),
        Link::Stored(_) => unimplemented!("Link::Stored"),
    }
}

fn load_mut(link: &mut Link) -> Result<&mut Node, MastError> {
    match link {
        // Link::Empty => Ok(Cow::Borrowed(&Node::empty()).to_mut()),
        Link::MutableNode(ref mut node, _) => Ok(node),
        Link::SharedNode(ref mut rc) => {
            let mutable = Rc::make_mut(rc).to_owned();
            *link = Link::MutableNode(mutable, Some(rc.clone()));
            if let Link::MutableNode(ref mut scopey, _) = link {
                Ok(scopey)
            } else {
                panic!("asdf")
            }
        }
        Link::Stored(_) => unimplemented!("Link::Stored"),
    }
}

// struct NodeAndSlot<'a>(&'a mut Node<'a>, usize);
/*
struct FindOptions<'a> {
    mast: &'a mut Mast<'a>,
    target_layer: u8,
    current_height: u8,
    create_missing_nodes: bool,
    node_path: Vec<&'a mut Node>,
    link_path: Vec<usize>,
}
*/
impl Node {
    fn new(branch_factor: usize) -> Node {
        let mut link = Vec::with_capacity(branch_factor + 1);
        link.push(None);
        Node {
            key: Vec::with_capacity(branch_factor),
            value: Vec::with_capacity(branch_factor),
            link,
            dirty: false,
        }
    }
    /*
    fn follow(
        &'a mut self,
        index: usize,
        create_ok: bool,
        m: &'a mut Mast<'a>,
    ) -> std::result::Result<&'a mut Node<'a>, std::io::Error> {
        if let Some(ref mut links) = self.link {
            return Ok(m.load(&mut links[index])?);
        } else if !create_ok {
            return Ok(self);
        }
        return Ok(&mut Node::empty());
    }*/
    fn insert(
        &mut self,
        key: i32,
        value: i32,
        distance: u8,
        key_order: fn(&i32, &i32) -> i8,
    ) -> Result<InsertResult, MastError> {
        let (equal, i) = get_index_for_key(&key, &self.key, key_order);
        if distance != 0 {
            let mut z = self.link.get_mut(i).unwrap();
            let child = match &mut z {
                Some(ref mut link) => load_mut(link)?,
                None => {
                    *z = Some(Link::MutableNode(Node::new(self.key.capacity()), None));
                    match &mut z {
                        Some(ref mut link) => load_mut(link)?,
                        None => panic!("can't load just-set link"),
                    }
                }
            };
            let res = child.insert(key, value, distance - 1, key_order)?;
            match res {
                InsertResult::NoChange => (),
                _ => self.dirty = true,
            };
            return Ok(res);
        }

        if equal {
            if value == self.value[i] {
                return Ok(InsertResult::NoChange);
            }
            self.value[i] = value;
            self.dirty = true;
            return Ok(InsertResult::Updated);
        }

        let (left_link, right_link) = match self.link.get_mut(i).unwrap() {
            Some(ref mut link) => {
                let child = load_mut(link)?;
                split(child, &key, key_order)?
            }
            None => (None, None),
        };

        self.key.insert(i, key);
        self.value.insert(i, value);
        self.link[i] = right_link;
        self.link.insert(i, left_link);
        self.dirty = true;

        return Ok(InsertResult::Inserted);
    }

    fn can_grow(
        &self,
        current_height: u8,
        key_layer: fn(&i32, u16) -> u8,
        branch_factor: u16,
    ) -> bool {
        for key in &self.key {
            if key_layer(key, branch_factor) > current_height {
                return true;
            }
        }
        return false;
    }

    fn grow(
        &mut self,
        current_height: u8,
        key_layer: fn(&i32, u16) -> u8,
        branch_factor: u16,
    ) -> Option<Link> {
        let mut new_parent = Node::new(self.key.capacity());
        if !self.is_empty() {
        for i in 0..self.key.len() {
            let key = &self.key[i];
            let layer = key_layer(key, branch_factor);
            if layer <= current_height {
                continue;
            }
            let new_left = self.extract(i);
            new_parent.key.push(self.key[0]);
            new_parent.value.push(self.value[0]);
            new_parent.link.insert(new_parent.link.len() - 1, new_left);
        }
        }
        let new_right = self.extract(self.key.len());
        *new_parent.link.last_mut().unwrap() = new_right;
        return new_parent.to_link();
    }

    fn extract(&mut self, end: usize) -> Option<Link> {
        let mut node = Node::new(self.key.capacity());
        node.key = self.key.drain(..end).collect();
        node.key.reserve(self.key.capacity());
        node.value = self.value.drain(..end).collect();
        node.value.reserve(self.key.capacity());
        node.link = self.link.drain(..=end).collect();
        node.link.reserve(self.key.capacity() + 1);
        self.link.insert(0, None);
        return node.to_link();
    }

    fn to_link(self) -> Option<Link> {
        if self.is_empty() {
            return None;
        }
        return Some(Link::MutableNode(self, None));
    }

    fn is_empty(&self) -> bool {
        return self.key.len() == 0
            && self.value.len() == 0
            && self.link.len() == 1
            && self.link[0].is_none();
    }
}
#[derive(Debug)]
enum InsertResult {
    Updated,
    Inserted,
    NoChange,
}
fn split(
    node: &mut Node,
    key: &i32,
    key_order: fn(&i32, &i32) -> i8,
) -> Result<(Option<Link>, Option<Link>), MastError> {
    let (equal, i) = get_index_for_key(key, &node.key, key_order);
    if equal {
        panic!("split not expecting existing key")
    }
    let mut left_node = Node::new(node.key.capacity());
    let mut right_node = Node::new(node.key.capacity());
    let (mut left, mut right) = node.key.split_at(i);
    left_node.key.extend_from_slice(left);
    right_node.key.extend_from_slice(right);
    let (mut left, mut right) = node.value.split_at(i);
    left_node.value.extend_from_slice(left);
    right_node.value.extend_from_slice(left);
    let (mut left, mut right) = node.link.split_at(i + 1);
    left_node.link.remove(0);
    left_node.link.extend_from_slice(left);
    right_node.link.extend_from_slice(right);

    // repartition left and right subtrees based on new key
    if let Some(ref mut cur_left_max_link) = left_node.link[i] {
        let left_max = load_mut(cur_left_max_link)?;
        let (left_max_link, too_big_link) = split(left_max, key, key_order)?;
        left_node.link[i] = left_max_link;
        right_node.link[0] = too_big_link;
    };
    if let Some(ref mut cur_right_min_link) = right_node.link[0] {
        let right_min = load_mut(cur_right_min_link)?;
        let (too_small_link, right_min_link) = split(right_min, key, key_order)?;
        if too_small_link.is_some() {
            panic!("bad news!")
        }
        right_node.link[0] = right_min_link
    };
    return Ok((left_node.to_link(), right_node.to_link()));
}

fn get_index_for_key(key: &i32, keys: &Vec<i32>, key_order: fn(&i32, &i32) -> i8) -> (bool, usize) {
    match keys.binary_search_by(|x| {
        let r = key_order(x, key);
        if r < 0 {
            Ordering::Less
        } else if r > 0 {
            Ordering::Greater
        } else {
            Ordering::Equal
        }
    }) {
        Ok(n) => (true, n),
        Err(n) => (false, n),
    }
}

fn bad_get_index_for_key(
    key: &i32,
    keys: &Vec<i32>,
    key_order: fn(&i32, &i32) -> i8,
) -> (bool, usize) {
    let mut cmp: i8 = 1;
    let mut i: usize = 0;
    while i < keys.len() {
        cmp = (key_order)(&keys[i], key);
        if cmp >= 0 {
            break;
        };
        i += 1
    }
    return (cmp == 0, i);
}
/*
fn findNode<'a>(key: i32, options: &mut FindOptions<'a>) -> std::result::Result<(), MastError> {
    let mut cmp: i8 = 1;
    let mut i: usize = 0;

    let keyOrder = options.mast.keyOrder;
    let mut node = options.node_path.last().unwrap();
    unimplemented!();
    while i < node.key.len() {
        cmp = (keyOrder)(node.key[i], key);
        if cmp >= 0 {
            break;
        }
        i += 1
    }
    if cmp == 0 || options.current_height == options.target_layer {
        return Ok(());
    };

    let child_link = match node.link {
        None => return Err(MastError::InvalidNode),
        Some(ref mut link) => link.get_mut(i).unwrap(),
    };
    let child = load(child_link)?;
    options.current_height -= 1;
    options.node_path.push(child);
    options.link_path.push(i);
    return findNode(key, options);
}*/
/*
trait NodeStore<'a> {
    fn load(&mut self, link: &'a mut Link) -> Result<&'a mut Node, std::io::Error>;
}

struct InMemoryNodeStore<'a> {
    map: std::collections::HashMap<String, Node>,
}
impl<'a> InMemoryNodeStore<'a> {
    fn new() -> InMemoryNodeStore<'a> {
        InMemoryNodeStore {
            map: std::collections::HashMap::new(),
        }
    }
}
impl<'a> NodeStore<'a> for InMemoryNodeStore<'a> {
    fn load(&mut self, link: &'a mut Link) -> Result<&'a mut Node, std::io::Error> {
        match link {
            // Link::Empty => Ok(Cow::Borrowed(&Node::empty()).to_mut()),
            Link::MutableNode(ref mut cow) => Ok(cow.to_mut()),
        }
    }
}
*/

#[test]
fn test_insert_accessibility() -> std::result::Result<(), MastError> {
    let mut t = Mast::newInMemory();

    let n = 16 * 16 + 2;
    for i in 0..n {
        t.insert(i, i)?;
        for i in 0..=i {
            let v = t.get(&i)?;
            assert_eq!(v, Some(&i))
        }
    }
    Ok(())
}

#[test]
fn test_bench_insert() -> std::result::Result<(), MastError> {
    let mut t = Mast::newInMemory();
    let parts = 4;

    let mut n = 16 * 16 * 16;
    let mut i = 0;
    let mut start = std::time::Instant::now();
    for p in 0..parts {
        while i < n {
            t.insert(i, i)?;
            i += 1;
        }
        let end = std::time::Instant::now();
        let diff = end - start;
        println!(
            "part {}/{}: height:{}, {}/s ({}ns/op) size:{}",
            p + 1,
            parts,
            //diff.as_micros(), // {}μs,
            t.height,
            1_000_000_000 / (diff.as_nanos() / t.size as u128),
            diff.as_nanos() / t.size as u128,
            t.size,
        );
        n *= 16;
        start = end;
    }

    Ok(())
}

#[test]
fn test_int_layer() {
    assert_eq!(default_layer(&-528, 16), 1);
    assert_eq!(default_layer(&-513, 16), 0);
    assert_eq!(default_layer(&-512, 16), 2);
    assert_eq!(default_layer(&-256, 16), 2);
    assert_eq!(default_layer(&-16, 16), 1);
    assert_eq!(default_layer(&-1, 16), 0);
    assert_eq!(default_layer(&0, 16), 0);
    assert_eq!(default_layer(&1, 16), 0);
    assert_eq!(default_layer(&16, 16), 1);
    assert_eq!(default_layer(&32, 16), 1);
}
