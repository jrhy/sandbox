extern crate sodiumoxide;

// TODO bring in a rust-native argon2
use sodiumoxide::crypto::pwhash::argon2id13;
use sodiumoxide::crypto::*;
use sodiumoxide::*;

use xsalsa20poly1305::aead::{generic_array::GenericArray, Aead, NewAead};
use xsalsa20poly1305::XSalsa20Poly1305;

extern crate error_chain;
use crate::errors::*;

use std::*;

const NONCE_LEN: usize = 24;
const KEY_LEN: usize = 32;
const MAC_LEN: usize = 16;
const DERIVEKEY_SALT_LEN: usize = 16;
const DERIVEKEY_OPS_LIMIT: argon2id13::OpsLimit = argon2id13::OpsLimit(1);
const DERIVEKEY_MEM_LIMIT: argon2id13::MemLimit = argon2id13::MemLimit(8192);

pub fn keyed_nonce_size(key: &[u8], plaintext: &[u8], desired_size: usize) -> Result<Vec<u8>> {
    let mut state = crypto::generichash::State::new(Some(desired_size), None).unwrap();
    state.update(plaintext).unwrap();
    state.update(key).unwrap();
    let digest = state.finalize().unwrap();
    let slice = digest.as_ref();
    if slice.len() == desired_size {
        Ok(slice.to_owned())
    } else {
        Err(format!("expected nonce len {}, got {}", desired_size, slice.len()).into())
    }
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
        .chain_err(|| "decryption")
}

pub fn derive_key(master_key: &[u8], context: &[u8]) -> Result<Vec<u8>> {
    let encoded = base64::encode(master_key, base64::Variant::Original);
    let pw = encoded.as_bytes();
    let salt = match argon2id13::Salt::from_slice(&keyed_nonce_size(
        master_key,
        context,
        DERIVEKEY_SALT_LEN,
    )?) {
        Some(salt) => salt,
        None => return Err("derive_key no salt".into()),
    };
    let mut k = [0; secretbox::KEYBYTES];
    match argon2id13::derive_key(&mut k, pw, &salt, DERIVEKEY_OPS_LIMIT, DERIVEKEY_MEM_LIMIT) {
        Ok(derived) => Ok(derived.to_owned()),
        Err(x) => return Err(format!("derive_key failure: {:?}", x).into()),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn nonce_reference() {
        let key = base64::decode(
            "UdHBz8klP8ze+cl+qP2zcFBOW952mo8DUc/tn59h6Rw=",
            base64::Variant::Original,
        )
        .unwrap();
        assert_eq!(
            "DuO9oCKfeLUrcIImvVH88Y67un3CFnRw",
            base64::encode(
                &keyed_nonce(&key, b"asdf").unwrap(),
                base64::Variant::Original
            )
        )
    }

    #[test]
    fn encrypt_reference() {
        let key = base64::decode(
            "UdHBz8klP8ze+cl+qP2zcFBOW952mo8DUc/tn59h6Rw=",
            base64::Variant::Original,
        )
        .unwrap();
        assert_eq!(
            "DuO9oCKfeLUrcIImvVH88Y67un3CFnRwhZOvsmKMKFjTuKYsiLv0bwSBbjo=",
            base64::encode(&encrypt(&key, b"asdf").unwrap(), base64::Variant::Original)
        )
    }

    #[test]
    fn decrypt_reference() {
        let key = base64::decode(
            "UdHBz8klP8ze+cl+qP2zcFBOW952mo8DUc/tn59h6Rw=",
            base64::Variant::Original,
        )
        .unwrap();
        assert_eq!(
            b"asdf".to_vec(),
            decrypt(
                &key,
                &base64::decode(
                    "l9+HCAKhVy0HhB9QLX07wX3QXJ0unVyUnhw1LktsDQ4cOzeCIhDrQk/RYVo=",
                    base64::Variant::Original
                )
                .unwrap()
            )
            .unwrap()
        )
    }
}
