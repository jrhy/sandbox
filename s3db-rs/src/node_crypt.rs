use anyhow::{anyhow, bail, Result};
use crypto_secretbox::aead::{generic_array::GenericArray, Aead, KeyInit};
use crypto_secretbox::XSalsa20Poly1305;

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
        bail!("expected {}-byte key, got {}", KEY_LEN, key.len());
    }
    let mut nonce = keyed_nonce(key, plaintext)?;

    let key = GenericArray::from_slice(key);
    let mut ciphertext = XSalsa20Poly1305::new(key)
        .encrypt(GenericArray::from_slice(&nonce), plaintext)
        .map_err(|_| anyhow!("XSalsa20Poly1305 encrypt"))?;

    let mut res: Vec<u8> = Vec::new();
    res.append(&mut nonce);
    res.append(&mut ciphertext);
    Ok(res)
}

pub fn decrypt(key: &[u8], ciphertext: &[u8]) -> Result<Vec<u8>> {
    if key.len() != KEY_LEN {
        bail!("expected {}-byte key, got {}", KEY_LEN, key.len());
    }
    if ciphertext.len() < NONCE_LEN {
        bail!("ciphertext too short to include {}-byte nonce", NONCE_LEN);
    }
    if ciphertext.len() < MAC_LEN {
        bail!("ciphertext too short to include {}-byte MAC", MAC_LEN);
    }
    let nonce = GenericArray::from_slice(&ciphertext[0..NONCE_LEN]);
    let ciphertext = &ciphertext[NONCE_LEN..ciphertext.len()];
    let key = GenericArray::from_slice(key);
    XSalsa20Poly1305::new(key)
        .decrypt(nonce, ciphertext)
        .map_err(|_| anyhow!("decrypt"))
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

    use base64::engine::general_purpose::STANDARD;
    use base64::Engine;

    let pw = STANDARD.encode(&combined);
    let pw = pw.as_bytes();
    match argon2::hash_raw(pw, salt, &config) {
        Ok(derived) => Ok(derived),
        Err(x) => return Err(anyhow!("derive_key failure: {:?}", x)),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use base64::engine::general_purpose::STANDARD;
    use base64::Engine;

    #[test]
    fn nonce_reference() {
        let key = STANDARD
            .decode("UdHBz8klP8ze+cl+qP2zcFBOW952mo8DUc/tn59h6Rw=")
            .unwrap();
        assert_eq!(
            "DuO9oCKfeLUrcIImvVH88Y67un3CFnRw",
            STANDARD.encode(&keyed_nonce(&key, b"asdf").unwrap())
        )
    }

    #[test]
    fn derive_reference() {
        let key = STANDARD
            .decode("UdHBz8klP8ze+cl+qP2zcFBOW952mo8DUc/tn59h6Rw=")
            .unwrap();
        let derived = derive_key(&key, b"foo").unwrap();
        assert_eq!(
            "7a0p0qOL3IqOBMPwjUlGokjz8FNDQDedZRXom5ii/Ls=",
            STANDARD.encode(&derived)
        )
    }

    #[test]
    fn encrypt_reference() {
        let key = STANDARD
            .decode("UdHBz8klP8ze+cl+qP2zcFBOW952mo8DUc/tn59h6Rw=")
            .unwrap();
        assert_eq!(
            "DuO9oCKfeLUrcIImvVH88Y67un3CFnRwhZOvsmKMKFjTuKYsiLv0bwSBbjo=",
            STANDARD.encode(&encrypt(&key, b"asdf").unwrap())
        )
    }

    #[test]
    fn decrypt_reference() {
        let key = STANDARD
            .decode("UdHBz8klP8ze+cl+qP2zcFBOW952mo8DUc/tn59h6Rw=")
            .unwrap();
        assert_eq!(
            b"asdf".to_vec(),
            decrypt(
                &key,
                &STANDARD
                    .decode("l9+HCAKhVy0HhB9QLX07wX3QXJ0unVyUnhw1LktsDQ4cOzeCIhDrQk/RYVo=")
                    .unwrap()
            )
            .unwrap()
        )
    }
}
