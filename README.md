ConccurrentDownload [![archiver GoDoc](https://img.shields.io/badge/reference-godoc-blue.svg?style=flat-square)](https://godoc.org/github.com/heindl/concurrent_download)
========

Package concurrent_download is a fast downloader for large files on servers that "Accept-Ranges" in bytes. It separates the download into multiple goroutines, recombines the bytes, compiles into the given output directory.
 

Currently supported formats/extensions:
- .txt
- .tar.gz
__Additional formats are trivial with archiver (github.com/mholt/archiver) but haven't been added yet.__


## Install

```bash
go get github.com/heindl/concurrent_download/cmd/concurrent_download
```

## Command Use

Download new file:

```bash
$ concurrent_download --routines=20 --url=http://www1.ncdc.noaa.gov/pub/data/ghcn/daily/ghcnd_all.tar.gz --path=~/Desktop/ghcnd/
```

## Library Use

```go
import "github.com/heindl/concurrent_download"
```

```go
downloader, err := concurrent_download.NewDownloader("http://www1.ncdc.noaa.gov/pub/data/ghcn/daily/ghcnd_all.tar.gz", "/Users/Desktop/ghcnd", 20)
```

```go
err := downloader.Download()
```