package main

import (
	"mp4"
	"fmt"
	"flag"
	"os"
)

var inputFile string
var f mp4.File

func init() {
	flag.StringVar(&inputFile, "i", "", "-i input_file.mp4")
	flag.Parse()
}

func main() {
	if inputFile == "" {
		flag.Usage()
		return
	}
	f, err := mp4.Open(inputFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	defer f.Close()

}
