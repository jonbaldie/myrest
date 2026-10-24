package main

import (
	"fmt"
	"os"

	"github.com/jonbaldie/myrest/internal/mysqltest"
)

func main() {
	if err := os.Setenv("MYREST_MYSQL_HARNESS_PORT", os.Args[1]); err != nil {
		panic(err)
	}
	harness, err := mysqltest.Start()
	if err != nil {
		panic(err)
	}
	defer harness.Stop()
	if err := harness.LoadSQL(os.Args[2]); err != nil {
		panic(err)
	}
	fmt.Println("fixtures loaded")
}
