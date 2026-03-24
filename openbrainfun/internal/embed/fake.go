package embed

import (
	"context"
	"fmt"
	"hash/fnv"
)

type Fake struct {
	vectors     map[string][]float32
	dimensions  int
	fingerprint string
}

func NewFake(vectors map[string][]float32) *Fake {
	copied := make(map[string][]float32, len(vectors))
	dimensions := 0
	for key, value := range vectors {
		vector := append([]float32(nil), value...)
		copied[key] = vector
		if len(vector) > dimensions {
			dimensions = len(vector)
		}
	}
	if dimensions == 0 {
		dimensions = 3
	}
	return &Fake{vectors: copied, dimensions: dimensions, fingerprint: fmt.Sprintf("fake|dim:%d|embed:v1", dimensions)}
}

func (f *Fake) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(inputs))
	for _, input := range inputs {
		if vector, ok := f.vectors[input]; ok {
			vectors = append(vectors, pad(vector, f.dimensions))
			continue
		}
		vectors = append(vectors, hashVector(input, f.dimensions))
	}
	return vectors, nil
}

func (f *Fake) Dimensions() int { return f.dimensions }

func (f *Fake) Model() string { return "fake" }

func (f *Fake) Fingerprint() string { return f.fingerprint }

func pad(values []float32, dimensions int) []float32 {
	result := make([]float32, dimensions)
	copy(result, values)
	return result
}

func hashVector(input string, dimensions int) []float32 {
	result := make([]float32, dimensions)
	h := fnv.New32a()
	_, _ = h.Write([]byte(input))
	index := int(h.Sum32() % uint32(dimensions))
	result[index] = 1
	return result
}
