extern crate bytes;
extern crate serde;
use std::convert::TryFrom;

const MAX_VARINT_LEN_64: usize = 10;

pub fn read(bytes: &bytes::Bytes) -> (u64, i32) {
    let mut i: usize = 0;
    let mut x: u64 = 0;
    let mut s: u32 = 0;
    while i < bytes.len() {
        let b = bytes[i];
        if i == MAX_VARINT_LEN_64 {
            return (0, -(i32::try_from(i).unwrap() + 1));
        }

        if b < 0x80 {
            if i == MAX_VARINT_LEN_64 - 1 && b > 1 {
                return (0, -(i32::try_from(i).unwrap() + 1)); // overflow
            }
            return (x | u64::from(b) << s, i32::try_from(i).unwrap() + 1);
        }
        x |= u64::from(b & 0x7f) << s;
        s += 7;

        i += 1;
    }
    (0, 0)
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn test_read() {
        let expected: (u64, i32) = (1, 1);
        let a = &[1u8];
        let actual: (u64, i32) = read(&bytes::Bytes::from_static(a));
        assert_eq!(expected, actual)
    }
}
