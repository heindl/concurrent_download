package concurrent_download

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func BenchmarkDownload(b *testing.B) {
	Convey("should download and save large remote file", b, func() {
		downloader, err := NewDownloader("http://www1.ncdc.noaa.gov/pub/data/ghcn/daily/ghcnd-stations.txt", "/Users/m/Desktop/ghcnd/ghcnd-stations.txt", 10)
		So(err, ShouldBeNil)
		So(downloader.Download(), ShouldBeNil)
	})
}
