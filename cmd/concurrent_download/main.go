package main

import (
	"flag"
	"github.com/heindl/concurrent_download"
)

func main() {

	url := flag.String("url", "", "The url to find the file.")
	path := flag.String("path", "", "The location to save the file.")
	routines := flag.Int("routines", 10, "Number of concurrent go routines to use to download the file.")
	flag.Parse()

	s, err := concurrent_download.NewDownloader(*url, *path, *routines)
	if err != nil {
		panic(err)
	}
	if err := s.Download(); err != nil {
		panic(err)
	}
}