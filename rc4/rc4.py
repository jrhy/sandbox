#!/usr/bin/env python3

"""RC4 stream cipher implementation.

When using an alphabet, plaintext and ciphertext consist of the alphabet characters only.
In byte mode, plaintext and ciphertext are standard ASCII/Unicode strings.
"""


class RC4:
    def __init__(self, key, alphabet=None, mode="auto"):
        """Initialize RC4 cipher.

        Args:
            key: Key material (string or bytes).
            alphabet: Optional tuple/list/set of characters to use.
            mode: 'byte' for standard mode, 'alpha' for alphabet mode, 'auto' for default.

        Raises:
            ValueError: If alphabet is empty, has duplicates, or exceeds 256 chars.
        """

        def _to_bytes(k):
            if isinstance(k, str):
                return k.encode("utf-8")
            return bytes(k)

        self.key_bytes = _to_bytes(key)[:256]

        if alphabet is not None or mode == "alpha":
            if mode == "auto":
                mode = "alpha"

            if not alphabet:
                raise ValueError("Alphabet must be non-empty for alphabetic mode")

            self.alphabet = list(alphabet)
            self.alphabet_size = len(self.alphabet)
            self.use_alphabetic_mode = True

            if self.alphabet_size == 0:
                raise ValueError("Alphabet must be non-empty")

            if len(self.alphabet) != len(set(self.alphabet)):
                raise ValueError("Alphabet characters must be unique")

            if self.alphabet_size > 256:
                raise ValueError("Alphabet size must not exceed 256")

            self._char_to_idx = {c: i for i, c in enumerate(self.alphabet)}
            self._idx_to_char = {i: c for i, c in enumerate(self.alphabet)}
        else:
            self.alphabet = None
            self.alphabet_size = 256
            self._char_to_idx = None
            self._idx_to_char = None
            self.use_alphabetic_mode = False

        self.S = list(range(256))

        # Key Scheduling Algorithm (KSA)
        self._ksa()

        # Internal state
        self._i = 0
        self._j = 0

    def _ksa(self):
        """Key Scheduling Algorithm (KSA)."""
        j = 0
        key_len = len(self.key_bytes)
        for i in range(256):
            j = (j + self.S[i] + self.key_bytes[i % key_len]) % 256
            self.S[i], self.S[j] = self.S[j], self.S[i]

    def _advance(self):
        """Get the next keystream byte.

        Follows RC4 stream:
        i = (i + 1) mod 256
        j = (j + S[i]) mod 256
        swap(S[i], S[j])
        return S[(i + S[j]) mod 256]
        """
        # Step 1: i = (i + 1) mod 256
        self._i = (self._i + 1) % 256

        # Step 2: j = (j + S[i]) mod 256 (using NEW i from step 1)
        self._j = (self._j + self.S[self._i]) % 256

        # Step 3: swap(S[i], S[j])
        self.S[self._i], self.S[self._j] = self.S[self._j], self.S[self._i]

        # Step 4: return S[(i + S[j]) mod 256]
        return self.S[(self._i + self.S[self._j]) % 256]

    def _reset_state(self):
        """Reset internal state after KSA - for fresh keystream."""
        # Reset S-box to initial state [0, 1, 2, ..., 255]
        self.S = list(range(256))
        # Reset indices
        self._i = 0
        self._j = 0
        # Run KSA
        self._ksa()

    def generate_keystream(self, n):
        """Generate n keystream bytes.

        Args:
            n: Number of bytes to generate.

        Returns:
            List of keystream bytes.
        """
        return [self._advance() for _ in range(n)]

    def _process_data(self, data, direction):
        """Process data (bytes or str) using RC4.

        Args:
            data: Data to process (bytes or str).
            direction: 1 for encryption, -1 for decryption.

        Returns:
            Processed data as bytes or str.

        Raises:
            ValueError: If character/byte is not in alphabet (alphabetic mode).
        """
        self._reset_state()

        if isinstance(data, bytes):
            res = bytearray()
            for byte in data:
                res.append(byte ^ self._advance())
            return bytes(res)

        if isinstance(data, str):
            if self.use_alphabetic_mode:
                res = []
                for c in data:
                    if c not in self._char_to_idx:
                        raise ValueError(f"Character '{c}' not in alphabet")
                    index = self._char_to_idx[c]
                    new_index = (index + direction * self._advance()) % self.alphabet_size
                    res.append(self._idx_to_char[new_index])
                return "".join(res)
            else:
                return "".join(chr(ord(c) ^ self._advance()) for c in data)

        raise TypeError("Data must be bytes or str")

    def encrypt(self, plaintext):
        """Encrypt plaintext.

        Args:
            plaintext: String or bytes to encrypt.

        Returns:
            Encrypted string or bytes.

        Raises:
            ValueError: If plaintext contains characters not in alphabet (alphabetic mode).
        """
        return self._process_data(plaintext, 1)

    def decrypt(self, ciphertext):
        """Decrypt ciphertext.

        Args:
            ciphertext: String or bytes to decrypt.

        Returns:
            Decrypted string or bytes.

        Raises:
            ValueError: If ciphertext contains characters not in alphabet (alphabetic mode).
        """
        return self._process_data(ciphertext, -1)

    def __str__(self):
        res = f'S: [{",".join(hex(num) for num in self.S[:16])}]\n'
        res += f'K: [{",".join(hex(num) for num in self.key_bytes[:10])}]\n\n'

        if self.use_alphabetic_mode:
            res += f"Alphabet: [{', '.join(repr(c) for c in self.alphabet)}]\n"
            res += f"Alphabet size: {self.alphabet_size}"

        res += (
            "\nAlphabet mode: enabled"
            if self.use_alphabetic_mode
            else "Alphabet mode: disabled"
        )
        return res


if __name__ == "__main__":
    # Test basic functionality
    r = RC4("Key")
    print("Byte mode RC4 initialized")
    print(f"S-box: {r.S[:32]}")
    ct = r.encrypt("Hello")
    print(f"Encrypted 'Hello': {ct}")
    pt = r.decrypt(ct)
    print(f"Decrypted: {pt}")

    print("\n--- Alphabet mode ---")
    cipher = RC4("Key", alphabet="0123456789")
    ct_alpha = cipher.encrypt("123")
    print(f"Encrypted '123' (alpha): {ct_alpha}")
    pt_alpha = cipher.decrypt(ct_alpha)
    print(f"Decrypted: {pt_alpha}")

    print("\n--- Key generation test ---")
    r2 = RC4("Key")
    ks1 = r2.generate_keystream(5)
    r2._reset_state()
    ks2 = r2.generate_keystream(5)
    print(f"Same keystream: {ks1 == ks2}")
