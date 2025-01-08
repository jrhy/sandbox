package rc44csp

import (
	"sync"
)

type RC4 struct {
	S         [256]byte
	generator chan byte
	want      chan struct{}
	wg        sync.WaitGroup
}

func New(k []byte) *RC4 {
	r := &RC4{
		want:      make(chan struct{}, 1),
		generator: make(chan byte, 1),
	}

	// KSA
	var j byte
	for i := 0; i < 256; i++ {
		r.S[i] = byte(i)
	}
	for i := 0; i < 256; i++ {
		j += r.S[i] + k[i%len(k)]
		r.S[i], r.S[j] = r.S[j], r.S[i]
	}

	// PRGA
	r.wg.Add(1)
	go func() {
		var i, j byte
		for {
			select {
			case _, ok := <-r.want:
				if !ok {
					r.wg.Done()
					return
				}
				i++
				j += r.S[i]
				r.S[i], r.S[j] = r.S[j], r.S[i]
				r.generator <- r.S[r.S[i]+r.S[j]]
			}
		}
	}()
	return r
}

func (r *RC4) Generate() byte {
	r.want <- struct{}{}
	return <-r.generator
}

func (r *RC4) Close() {
	if r.want == nil {
		return
	}
	close(r.want)
	r.want = nil
	close(r.generator)
	r.generator = nil
	r.wg.Wait()
}

func (r *RC4) Encrypt(s string) []byte {
	res := make([]byte, len(s))
	for i := range s {
		res[i] = r.Generate() ^ s[i]
	}
	return res
}
