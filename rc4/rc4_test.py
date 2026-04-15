import pytest
from rc4 import RC4

@pytest.fixture
def default_cipher():
    """Fixture providing a default RC4 instance (byte mode)."""
    return RC4("Key")

@pytest.fixture
def alpha_cipher_simple():
    """Fixture providing an RC4 instance with a simple numeric alphabet."""
    return RC4("key", alphabet="0123456789 !@#$%")

@pytest.fixture
def alpha_cipher_alphabet():
    """Fixture providing an RC4 instance with a full lowercase alphabet."""
    alphabet = "abcdefghijklmnopqrstuvwxyz"
    return RC4("key", alphabet=alphabet)

@pytest.fixture
def alpha_cipher_mixed():
    """Fixture providing an RC4 instance with mixed case and special chars."""
    alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz!#$%"
    return RC4("key", alphabet=alphabet)

# --- Byte Mode Tests ---

def test_byte_mode_basic(default_cipher):
    """Test standard byte mode (backward compatibility)."""
    pt = "P"
    ct = default_cipher.encrypt(pt)
    decrypted = default_cipher.decrypt(ct)
    assert pt == decrypted


def test_byte_mode_encrypt_decrypt(default_cipher):
    """Test encrypt/decrypt roundtrip in byte mode."""
    plaintext = "Hello World"
    ciphertext = default_cipher.encrypt(plaintext)
    decrypted = default_cipher.decrypt(ciphertext)
    assert plaintext == decrypted


def test_byte_mode_roundtrip(default_cipher):
    """Test multiple encrypt/decrypt cycles maintain state."""
    for msg in ["Hello", "World", "Test123", "!@#$%"]:
        c = default_cipher.encrypt(msg)
        d = default_cipher.decrypt(c)
        assert msg == d


def test_generate_keystream(default_cipher):
    """Test keystream generation works correctly."""
    ks = default_cipher.generate_keystream(10)
    assert len(ks) == 10


def test_str_representation(default_cipher):
    """Test __str__ representation."""
    r_str = str(default_cipher)
    assert "Alphabet mode: disabled" in r_str


# --- Alphabet Mode Tests ---

def test_alphabet_mode_basic(alpha_cipher_simple):
    """Test basic alphabet mode functionality."""
    assert alpha_cipher_simple.alphabet_size == 16


def test_alphabet_mode_basic_encrypt_decrypt(alpha_cipher_alphabet):
    """Test encrypt/decrypt with alphabetic chars."""
    for msg in ["hello", "world", "test"]:
        ciphertext = alpha_cipher_alphabet.encrypt(msg)
        decrypted = alpha_cipher_alphabet.decrypt(ciphertext)
        assert msg == decrypted
        for char in ciphertext:
            assert char in alpha_cipher_alphabet.alphabet


@pytest.mark.parametrize("msg", ["A!", "B#$", "Hello", "World"])
def test_alphabet_mode_mixed_case(alpha_cipher_mixed, msg):
    """Test encrypt/decrypt with mixed case letters."""
    ciphertext = alpha_cipher_mixed.encrypt(msg)
    decrypted = alpha_cipher_mixed.decrypt(ciphertext)
    assert msg == decrypted
    for char in ciphertext:
        assert char in alpha_cipher_mixed.alphabet


@pytest.mark.parametrize("msg", ["12345", "!@#$%", "01234"])
def test_alphabet_mode_encrypt_decrypt(alpha_cipher_simple, msg):
    """Test encrypt/decrypt with numeric and special chars."""
    ciphertext = alpha_cipher_simple.encrypt(msg)
    decrypted = alpha_cipher_simple.decrypt(ciphertext)
    assert msg == decrypted


@pytest.mark.parametrize("msg", ["12345", "67890", "01234"])
def test_alphabet_mode_numeric_only(msg):
    """Test encrypt/decrypt with numeric only."""
    # Note: Using a local instance here because it's a very specific test case
    r = RC4("key", alphabet="0123456789")
    ciphertext = r.encrypt(msg)
    decrypted = r.decrypt(ciphertext)
    assert msg == decrypted
    for char in ciphertext:
        assert char in r.alphabet


def test_generate_keystream_alphabetic(alpha_cipher_alphabet):
    """Test keystream generation in alphabetic mode returns raw bytes."""
    ks = alpha_cipher_alphabet.generate_keystream(10)
    assert len(ks) == 10
    # Keystream bytes are raw (0-255), mapping happens during encrypt/decrypt


def test_alphabet_property(alpha_cipher_alphabet):
    """Test that alphabet property is accessible."""
    assert alpha_cipher_alphabet.alphabet is not None
    assert len(alpha_cipher_alphabet.alphabet) == 26


def test_str_representation_alpha(alpha_cipher_simple):
    """Test __str__ representation for alphabet mode."""
    r_alpha_str = str(alpha_cipher_simple)
    assert "Alphabet mode: enabled" in r_alpha_str


def test_alphabet_mode_too_large():
    """Test that alphabet larger than 256 raises."""
    with pytest.raises(ValueError):
        RC4("key", alphabet=list(range(300)))


def test_alphabet_mode_empty():
    """Test that empty alphabet raises."""
    with pytest.raises(ValueError):
        RC4("key", alphabet=[])
