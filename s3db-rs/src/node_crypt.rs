extern crate sodiumoxide;

use sodiumoxide::*;
use sodiumoxide::crypto::pwhash::argon2id13;
use sodiumoxide::crypto::secretbox::xsalsa20poly1305::*;
use sodiumoxide::crypto::*;
use std::*;

const NONCE_LEN: usize = 24;
const KEY_LEN: usize = 32;
const MAC_LEN: usize = 16;
const DERIVEKEY_SALT_LEN: usize = 16;
const DERIVEKEY_OPS_LIMIT: argon2id13::OpsLimit = argon2id13::OpsLimit(1);
const DERIVEKEY_MEM_LIMIT: argon2id13::MemLimit = argon2id13::MemLimit(8192);

pub fn keyed_nonce_size(key: &[u8], plaintext: &[u8], desired_size: usize) -> Vec<u8> {
    let mut state = crypto::generichash::State::new(Some(desired_size), None).unwrap();
    state.update(plaintext).unwrap();
    state.update(key).unwrap();
    let digest = state.finalize().unwrap();
    let slice = digest.as_ref();
    return if slice.len() == desired_size {
        slice.to_owned()
    } else {
        panic!("expected nonce len {}, got {}",desired_size, slice.len())
    }
}

pub fn keyed_nonce(key: &[u8], plaintext: &[u8]) -> Vec<u8> {
    keyed_nonce_size(key, plaintext, NONCE_LEN)
}

pub fn encrypt(key: &[u8], plaintext: &[u8]) -> Vec<u8> {
    let mut nonce = keyed_nonce(key, plaintext);
    let mut ciphertext =
        crypto::secretbox::seal(
            plaintext,
            &Nonce::from_slice(&nonce).expect("failed making libsodium nonce"),
            &Key::from_slice(key).expect("failed making libsodium key"));
    let mut res: Vec<u8> = Vec::new();
    res.append(&mut nonce);
    res.append(&mut ciphertext);
    res
}

pub fn decrypt(key: &[u8], ciphertext: &[u8]) -> Vec<u8> {
    if key.len() != KEY_LEN {
        panic!("expected {}-byte key, got {}", KEY_LEN, key.len())
    }
    if ciphertext.len() < NONCE_LEN {
        panic!("ciphertext too short to include {}-byte nonce", NONCE_LEN)
    }
    let nonce = &ciphertext[0..NONCE_LEN];
    let ciphertext = &ciphertext[NONCE_LEN..ciphertext.len()];
    if ciphertext.len() < MAC_LEN {
        panic!("ciphertext too short to include {}-byte MAC", MAC_LEN)
    }
    let plaintext =
        crypto::secretbox::open(
            ciphertext,
            &Nonce::from_slice(nonce).expect("failed making libsodium nonce"),
            &Key::from_slice(key).expect("failed making libsodium key"));
    // XXX, do real errors
    match plaintext {
        Err(_) => panic!("decryption failed"),
        Ok(x) => x
    }
}

pub fn derive_key(master_key: &[u8], context: &[u8]) -> Vec<u8> {
    let encoded = base64::encode(master_key, base64::Variant::Original);
    let pw = encoded.as_bytes();
    let salt = argon2id13::Salt::from_slice(&keyed_nonce_size(master_key, context, DERIVEKEY_SALT_LEN)).unwrap();
    let mut k = [0; secretbox::KEYBYTES];
    let derived = argon2id13::derive_key(&mut k, pw, &salt,
                                         DERIVEKEY_OPS_LIMIT,
                                         DERIVEKEY_MEM_LIMIT);
    derived.unwrap().to_owned()
}

  #[cfg(test)]
  mod tests {
      use super::*;

#[test]
fn nonce_reference() {
    let key = base64::decode("UdHBz8klP8ze+cl+qP2zcFBOW952mo8DUc/tn59h6Rw=", base64::Variant::Original).unwrap();
    assert_eq!(
        "DuO9oCKfeLUrcIImvVH88Y67un3CFnRw",
        base64::encode(&keyed_nonce(&key, b"asdf"), base64::Variant::Original))
}

#[test]
fn encrypt_reference() {
    let key = base64::decode("UdHBz8klP8ze+cl+qP2zcFBOW952mo8DUc/tn59h6Rw=", base64::Variant::Original).unwrap();
    assert_eq!(
        "DuO9oCKfeLUrcIImvVH88Y67un3CFnRwhZOvsmKMKFjTuKYsiLv0bwSBbjo=",
        base64::encode(&encrypt(&key, b"asdf"), base64::Variant::Original))
}

#[test]
fn decrypt_reference() {
    let key = base64::decode("UdHBz8klP8ze+cl+qP2zcFBOW952mo8DUc/tn59h6Rw=", base64::Variant::Original).unwrap();
    assert_eq!(
        b"asdf".to_vec(),
        decrypt(
            &key,
            &base64::decode("l9+HCAKhVy0HhB9QLX07wX3QXJ0unVyUnhw1LktsDQ4cOzeCIhDrQk/RYVo=", base64::Variant::Original).unwrap()))
}

}

