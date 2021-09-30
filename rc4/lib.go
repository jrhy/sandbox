package rc4

// this is a terrible idea, but fun

type RC4 struct {
	s    []byte
	mod  int
	i, j byte
}

func New(key []byte, mod int) *RC4 {
	s := make([]byte, mod)
	if mod < 2 {
		panic("modulus too small")
	}
	if mod > 256 {
		panic("modulus too large for byte elements")
	}
	for i := 0; i < len(key); i++ {
		if int(key[i]) >= mod {
			panic("key element larger than modulus")
		}
	}
	for i := 0; i < mod; i++ {
		s[i] = byte(i)
	}
	j := 0
	for i := 0; i < mod; i++ {
		j = (j + int(s[i]) + int(key[i%len(key)])) % mod
		s[i], s[j] = s[j], s[i]
	}
	return &RC4{s: s, mod: mod}
}

func (r *RC4) Generate() byte {
	r.i = byte((int(r.i) + 1) % r.mod)
	r.j = byte((int(r.j) + int(r.s[r.i])) % r.mod)
	r.s[r.i], r.s[r.j] = r.s[r.j], r.s[r.i]
	return r.s[(int(r.s[r.i])+int(r.s[r.j]))%r.mod]
}

func (r *RC4) Drop(n int) *RC4 {
	for i := 0; i < n; i++ {
		r.Generate()
	}
	return r
}
