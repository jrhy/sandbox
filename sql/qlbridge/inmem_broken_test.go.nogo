package fun_test

import (
	"database/sql/driver"
	"fmt"
	"testing"

	"github.com/araddon/qlbridge/datasource/membtree"
	"github.com/araddon/qlbridge/datasource/mockcsvtestdata"
	"github.com/araddon/qlbridge/exec"
	"github.com/araddon/qlbridge/expr/builtins"
	"github.com/araddon/qlbridge/schema"
	"github.com/araddon/qlbridge/testutil"
)

func init() {
	exec.RegisterSqlDriver()
	exec.DisableRecover()
	builtins.LoadAllBuiltins()
	static := membtree.NewStaticDataSource("users", 0, nil, []string{"user_id", "name", "email", "created", "roles"})
	schema.RegisterSourceType("inmem_testsuite", static)

	mockcsvtestdata.SetContextToMockCsv()
	mockcsvtestdata.LoadTestDataOnce()

	fmt.Printf("init done\n")
}

func TestHello(t *testing.T) {
	testutil.TestSelect(t, `select null or true from users;`,
		[][]driver.Value{{}}, //int64(1), int64(2)},
		//{int64(3), int64(4)}},
	)
}
