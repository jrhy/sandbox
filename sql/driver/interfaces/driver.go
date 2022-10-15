package interfaces

import (
	"context"

	"github.com/jrhy/sandbox/sql/types"
)

type DB interface {
	CreateIndex(context.Context, string, string, *types.Schema, *types.Evaluator) (Index, error)
	DropIndex(context.Context, string) error
	GetIndex(context.Context, string) (Index, bool, error)
	CreateTable(context.Context, string, *types.Schema) (Table, error)
	DropTable(context.Context, string) error
	GetTable(context.Context, string) (Table, bool, error)
	//CreateView(context.Context, string, string, *types.Schema, bool) (View, error)
	//DropView(context.Context, string) error
	//GetView(context.Context, string) (View, bool, error)
}
type Index interface {
	Source
}
type Table interface {
	Insert(context.Context, []types.Row) error
	Source
}
type View interface {
	Source
}
type Source interface {
	Source() types.Source
}
