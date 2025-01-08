#!/usr/bin/env python3

class RC4:
  def __init__(self, key):
    self.S = list(range(256))
    self.i = 0
    self.j = 0
    self.K = [ord(char) if ord(char) <= 255 else (_ for _ in ()).throw(ValueError(f"Character '{char}' has an ASCII value greater than 255: {ord(char)}")) for char in key]
    self.ksa()
    self.i = 0
    self.j = 0

  def swap(self, i, j):
    t = self.getS(i)
    self.S[i % 256] = self.getS(j)
    self.S[j % 256] = t

  def ksa(self):
    j = 0
    for i in range(256):
      j = (j + self.getS(i) + self.K[i % len(self.K)]) % 256
      self.swap(i, j)

  def generate(self, n):
    res = []
    for _ in range(n):
      self.i = (self.i + 1) % 256
      self.j = (self.j + self.getS(self.i)) % 256
      self.swap(self.i, self.j)
      res.append(self.getSP(self.getSP(self.i) + self.getSP(self.j)))
    return res

  def __str__(self):
    res = "S: [" + (",".join(hex(num) for num in self.S)) + "]\n"
    res += "K: [" + (",".join(hex(num) for num in self.K)) + "]"
    return res
    
  def getS(self, n):
    if n < 256:
      return self.S[n] 
    else:
      throw(ValueError(f"Index '{n}' too large")) 

  def getSP(self, n):
    return self.S[n % 256] 

