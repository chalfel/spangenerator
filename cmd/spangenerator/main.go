package main

import (
	"flag"
	"log"

	"github.com/chalfel/spangenerator"
)

func main() {
	root := flag.String("root", ".", "Root directory to apply span injection")
	tracerName := flag.String("tracer-name", ".", "Name of the tracer to use")
	flag.Parse()

	if root == nil || *root == "" {
		log.Fatal("Root directory is required")
		return
	}

	if tracerName == nil || *tracerName == "" {
		log.Fatal("Tracer name is required")
		return
	}

	// Call the library function
	err := spangenerator.InjectSpans(*root, *tracerName)
	if err != nil {
		log.Fatal(err)
	}
}
