package main

import (
	"example.com/mishis4x/cmd"
	_ "github.com/go-sql-driver/mysql"
)

func main() {
	cmd.Execute()
}
