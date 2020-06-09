package main

import (
	"os"

	strife "github.com/dpatterbee/strife/src"
)

func main() {
	os.Exit(strife.Run(os.Args[1:]))
}
