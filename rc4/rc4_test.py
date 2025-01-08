import unittest
from rc4 import RC4

class TestVectors(unittest.TestCase):
  def test_first(self):
    r = RC4("\x01\x02\x03\x04\x05")
    self.assertEqual(r.K, [1, 2, 3, 4, 5])
    g = r.generate(16)
    gs = " ".join([hex(char)[2:].zfill(2) for char in g])
    self.assertEqual("b2 39 63 05 f0 3d c0 27 cc c3 52 4a 0a 11 18 a8", gs)

  def test_wikipedia(self):
    r = RC4("Key")
    g = r.generate(9)
    gs = " ".join([hex(char)[2:].zfill(2) for char in g[:2]])
    self.assertEqual("eb 9f", gs)
    p = "Plaintext"
    c = [g[i] ^ ord(p[i]) for i in range(len(p))]
    cs = " ".join([hex(char)[2:].upper().zfill(2) for char in c])
    self.assertEqual("BB F3 16 E8 D9 40 AF 0A D3", cs)

  def test_swap(self):
    r = RC4("Hello")
    s0 = r.S[0]
    s1 = r.S[1]
    r.swap(0,1)
    self.assertEqual(r.S[0], s1)
    self.assertEqual(r.S[1], s0)

if __name__ == '__main__':
    unittest.main()

