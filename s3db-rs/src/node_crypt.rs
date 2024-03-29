use xsalsa20poly1305::aead::{generic_array::GenericArray, Aead, NewAead};
use xsalsa20poly1305::XSalsa20Poly1305;

extern crate error_chain;
use crate::errors::*;

use std::*;

const NONCE_LEN: usize = 24;
const KEY_LEN: usize = 32;
const MAC_LEN: usize = 16;
const DERIVEKEY_SALT_LEN: usize = 16;
const DERIVEKEY_OPS_LIMIT: u32 = 1;
const DERIVEKEY_MEM_LIMIT: u32 = 8192;

pub fn keyed_nonce_size(key: &[u8], plaintext: &[u8], desired_size: usize) -> Result<Vec<u8>> {
    use blake2::digest::{Update, VariableOutput};
    use blake2::VarBlake2b;

    let mut hasher = VarBlake2b::new(desired_size).unwrap();
    hasher.update(plaintext);
    hasher.update(key);
    Ok(hasher.finalize_boxed().into())
}

pub fn keyed_nonce(key: &[u8], plaintext: &[u8]) -> Result<Vec<u8>> {
    keyed_nonce_size(key, plaintext, NONCE_LEN)
}

pub fn encrypt(key: &[u8], plaintext: &[u8]) -> Result<Vec<u8>> {
    if key.len() != KEY_LEN {
        return Err(format!("expected {}-byte key, got {}", KEY_LEN, key.len()).into());
    }
    let mut nonce = keyed_nonce(key, plaintext)?;

    let key = GenericArray::from_slice(key);
    let mut ciphertext = XSalsa20Poly1305::new(key)
        .encrypt(GenericArray::from_slice(&nonce), plaintext)
        .chain_err(|| "XSalsa20Poly1305 encrypt")?;

    let mut res: Vec<u8> = Vec::new();
    res.append(&mut nonce);
    res.append(&mut ciphertext);
    Ok(res)
}

pub fn decrypt(key: &[u8], ciphertext: &[u8]) -> Result<Vec<u8>> {
    if key.len() != KEY_LEN {
        return Err(format!("expected {}-byte key, got {}", KEY_LEN, key.len()).into());
    }
    if ciphertext.len() < NONCE_LEN {
        return Err(format!("ciphertext too short to include {}-byte nonce", NONCE_LEN).into());
    }
    if ciphertext.len() < MAC_LEN {
        return Err(format!("ciphertext too short to include {}-byte MAC", MAC_LEN).into());
    }
    let nonce = GenericArray::from_slice(&ciphertext[0..NONCE_LEN]);
    let ciphertext = &ciphertext[NONCE_LEN..ciphertext.len()];
    let key = GenericArray::from_slice(key);
    XSalsa20Poly1305::new(key)
        .decrypt(nonce, ciphertext)
        .chain_err(|| "decrypt")
}

pub fn derive_key(master_key: &[u8], context: &[u8]) -> Result<Vec<u8>> {
    use argon2::{Config, ThreadMode, Variant, Version};
    let mut combined: Vec<u8> = context.into();
    let mut vec: Vec<u8> = master_key.into();
    combined.append(&mut vec);

    let salt = &keyed_nonce_size(&combined, &[], DERIVEKEY_SALT_LEN)?;
    let config = Config {
        variant: Variant::Argon2id,
        version: Version::Version13,
        mem_cost: DERIVEKEY_MEM_LIMIT / 1024,
        time_cost: DERIVEKEY_OPS_LIMIT,
        lanes: 1,
        thread_mode: ThreadMode::Sequential,
        secret: &[],
        ad: &[],
        hash_length: KEY_LEN as u32,
    };

    let pw = base64::encode(&combined);
    let pw = pw.as_bytes();
    match argon2::hash_raw(pw, salt, &config) {
        Ok(derived) => Ok(derived),
        Err(x) => return Err(format!("derive_key failure: {:?}", x).into()),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn nonce_reference() {
        let key = base64::decode("UdHBz8klP8ze+cl+qP2zcFBOW952mo8DUc/tn59h6Rw=").unwrap();
        assert_eq!(
            "DuO9oCKfeLUrcIImvVH88Y67un3CFnRw",
            base64::encode(&keyed_nonce(&key, b"asdf").unwrap())
        )
    }

    #[test]
    fn derive_reference() {
        let key = base64::decode("UdHBz8klP8ze+cl+qP2zcFBOW952mo8DUc/tn59h6Rw=").unwrap();
        let derived = derive_key(&key, b"foo").unwrap();
        assert_eq!(
            "7a0p0qOL3IqOBMPwjUlGokjz8FNDQDedZRXom5ii/Ls=",
            base64::encode(&derived)
        )
    }

    #[test]
    fn encrypt_reference() {
        let key = base64::decode("UdHBz8klP8ze+cl+qP2zcFBOW952mo8DUc/tn59h6Rw=").unwrap();
        assert_eq!(
            "DuO9oCKfeLUrcIImvVH88Y67un3CFnRwhZOvsmKMKFjTuKYsiLv0bwSBbjo=",
            base64::encode(&encrypt(&key, b"asdf").unwrap())
        )
    }

    #[test]
    fn decrypt_reference() {
        let key = base64::decode("UdHBz8klP8ze+cl+qP2zcFBOW952mo8DUc/tn59h6Rw=").unwrap();
        assert_eq!(
            b"asdf".to_vec(),
            decrypt(
                &key,
                &base64::decode("l9+HCAKhVy0HhB9QLX07wX3QXJ0unVyUnhw1LktsDQ4cOzeCIhDrQk/RYVo=")
                    .unwrap()
            )
            .unwrap()
        )
    }
}
