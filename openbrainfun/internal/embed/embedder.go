package embed

import "context"

type Embedder interface {
	Embed(ctx context.Context, inputs []string) ([][]float32, error)
	Dimensions() int
	Model() string
}
