package concurrent_download

import (
	"strconv"
	"fmt"
	"os"
	"github.com/saleswise/errors/errors"
	"net/http"
	"gopkg.in/tomb.v2"
	"io"
	"sync"
	"github.com/gosuri/uiprogress"
	"io/ioutil"
	"strings"
	"sort"
	"time"
	"net"
	"path/filepath"
	"github.com/mholt/archiver"
)

const baseTempDownloadPath = "/tmp/godownload/"

func tempFileName(path string) string {
	return strings.Replace(path, "/", "_", -1) + "_"
}

var monitor struct {
	sync.Mutex
	current int
}

func NewDownloader(url, path string, routines int) (Downloader, error) {
	if strings.TrimSpace(url) == "" {
		return nil, errors.New("a file url is required to download")
	}
	if strings.TrimSpace(url) == "" {
		return nil, errors.New("a file url is required to download")
	}
	g := &downloader{
		FileURL: url,
		FinalPath: path,
		Routines: routines,
	}
	g.setMeasurements()
	return Downloader(g), nil
}

type Downloader interface{
	Download() error
}

type downloader struct {
	FileURL string
	TotalLength int
	SubsetLength int
	RemainingDiff int
	FinalPath string
	Routines int
	ProgressBar *uiprogress.Bar
}

func (this *downloader) setMeasurements() error {
	res, err := http.Head(this.FileURL); // 187 MB file of random numbers per line
	if err != nil {
		return errors.Wrap(err, "could not get response header")
	}
	this.TotalLength, err = strconv.Atoi(res.Header.Get("Content-Length")) // Get the content length from the header request
	if err != nil {
		return errors.Wrap(err, "could not get content length")
	}
	this.FileURL = res.Request.URL.String() // Should be the final URL after redirects.
	if res.Header.Get("Accept-Ranges") != "bytes" {
		fmt.Println("FileURL does not accept ranges so can not download concurrently.")
		this.Routines = 1
	}
	this.SubsetLength = this.TotalLength / this.Routines
	this.RemainingDiff = this.TotalLength % this.Routines
	return nil
}

func (this *downloader) Download() error {

	// Make the temp subdirectory
	if err := os.MkdirAll(baseTempDownloadPath, os.ModePerm); err != nil {
		return errors.Wrap(err, "could not make downloader directory")
	}
	uiprogress.Start()
	this.ProgressBar = uiprogress.AddBar(this.TotalLength/bytesToMegaBytes)
	this.ProgressBar.AppendCompleted()
	this.ProgressBar.AppendElapsed()
	tmb := tomb.Tomb{}
	tmb.Go(func()error {
		for _i := 0; _i < this.Routines; _i ++ {
			r := ranger{
				downloader: *this,
				CurrentSubset: _i,
				IsLastSubset: (_i == this.Routines - 1),
			}
			tmb.Go(r.download)
		}
		return nil
	})
	if err := tmb.Wait(); err != nil {
		return err
	}
	uiprogress.Stop()
	if err := this.concat(); err != nil {
		return err
	}
	return this.relocate()
}

func (this *downloader) concat() error {
	if this.Routines <= 1 {
		if err := os.Rename(
			baseTempDownloadPath + tempFileName(this.FinalPath)+"1",
			baseTempDownloadPath + tempFileName(this.FinalPath)+"combined",
		); err != nil {
			return errors.Wrap(err, "could not rename template files")
		}
		return nil
	}
	files, err := ioutil.ReadDir(baseTempDownloadPath)
	if err != nil {
		return errors.Wrap(err, "could not read temp directory")
	}
	var include temps
	for _, f := range files {
		if !strings.Contains(f.Name(), tempFileName(this.FinalPath)) {
			continue
		}
		if strings.HasSuffix(f.Name(), "combined") {
			continue
		}
		include = append(include, f.Name())
	}
	sort.Sort(include)
	combinedName := baseTempDownloadPath + tempFileName(this.FinalPath) + "_combined"
	combined, err := os.OpenFile(combinedName, os.O_APPEND|os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return errors.Wrap(err, "could not create new combined file")
	}
	defer combined.Close()
	for _, name := range include {
		if err := addToFile(combined, name); err != nil {
			return err
		}
	}
	return nil
}

func addToFile(combined *os.File, name string) error {
	f, err := os.OpenFile(baseTempDownloadPath + name, os.O_RDONLY, 0600)
	if err != nil {
		return errors.Wrap(err, "could not open file")
	}
	defer f.Close()
	if _, err := io.Copy(combined, f); err != nil {
		return errors.Wrap(err, "could not copy combined")
	}
	if err := os.Remove(baseTempDownloadPath + name); err != nil {
		return errors.Wrap(err, "could not remove file")
	}
	return nil
}

type Extension string
const (
	ExtensionText = ".txt"
	ExtensionTarGz = ".tar.gz"
)

func (this *downloader) relocate() error {
	fmt.Println(filepath.Ext(this.FileURL))
	combinedFileName := baseTempDownloadPath + tempFileName(this.FinalPath) + "_combined"
	// If it's a text file, we should just be able to rename and move.
	switch filepath.Ext(this.FileURL) {
	case ExtensionText:
		if err := os.MkdirAll(filepath.Dir(this.FinalPath), 0700); err != nil {
			return errors.Wrap(err, "could not create final output directories")
		}
		if err := os.Rename(
			combinedFileName,
			this.FinalPath,
		); err != nil {
			return errors.Wrap(err, "could not move temporary combined file into final path")
		}
	case ExtensionTarGz:
		if !archiver.TarGz.Match(combinedFileName) {
			return errors.New("not a valid .tar.gz file")
		}
		if err := os.MkdirAll(filepath.Dir(this.FinalPath), 0700); err != nil {
			return errors.Wrap(err, "could not create final output directories")
		}
		if err := archiver.TarGz.Open(combinedFileName, this.FinalPath); err != nil {
			return errors.Wrap(err, "could not open combined file name.")
		}
	default:
		return errors.Newf("extension %s not supported", filepath.Ext(this.FileURL))
	}

	return nil
}

type ranger struct {
	downloader
	CurrentSubset int
	IsLastSubset bool
	io.Reader
}

func (this *ranger) download() error {
	min := this.SubsetLength * this.CurrentSubset // Min range
	max := this.SubsetLength * (this.CurrentSubset + 1) // Max range
	if this.IsLastSubset {
		max += this.RemainingDiff // AddUsage the remaining bytes in the last request
	}
	client := http.DefaultClient
	client.Timeout = time.Minute * 10
	client.Transport = &http.Transport{
		Dial: (&net.Dialer{
			Timeout: time.Minute * 10,
		}).Dial,
		DisableKeepAlives: false,
		TLSHandshakeTimeout: time.Minute * 10,
	}
	req, err := http.NewRequest("GET", this.FileURL, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Accept-Encoding", "identity")
	req.Header.Add("Range", "bytes="+strconv.Itoa(int(min)) +"-" + strconv.Itoa(int(max)-1))
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	this.Reader = resp.Body
	out, err := os.Create(baseTempDownloadPath+tempFileName(this.FinalPath)+strconv.Itoa(this.CurrentSubset))
	if err != nil {
		return errors.Wrap(err, "could not create tmp file")
	}
	defer out.Close()
	if _, err := io.Copy(out, this); err != nil {
		return errors.Wrap(err, "could not copy data")
	}
	return nil
}

func (this *ranger) Read(p []byte) (int, error) {
	n, err := this.Reader.Read(p)
	monitor.Lock()
	defer monitor.Unlock()
	monitor.current += n

	// last read will have EOF err
	if err == nil || (err == io.EOF && n > 0) {
		this.ProgressBar.Set(monitor.current / bytesToMegaBytes)
	}

	return n, err
}

type temps []string
func (s temps) Len() int {
	return len(s)
}
func (s temps) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s temps) Less(i, j int) bool {
	il  := s[i][len(s[i])-1:]
	jl := s[j][len(s[j])-1:]
	return il < jl
}

var bytesToMegaBytes = 1048576