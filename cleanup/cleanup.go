package cleanup

import (
	"context"
	"log"
	"time"

	"github.com/andreimarcu/linx-server/backends/localfs"
	"github.com/andreimarcu/linx-server/expiry"
)

func Cleanup(filesDir string, metaDir string, noLogs bool) {
	fileBackend := localfs.NewLocalfsBackend(metaDir, filesDir, 0)

	files, err := fileBackend.List(context.Background())
	if err != nil {
		panic(err)
	}

	for _, filename := range files {
		metadata, err := fileBackend.Head(context.Background(), filename)
		if err != nil {
			if !noLogs {
				log.Printf("Failed to find metadata for %s", filename)
			}
		}

		if expiry.IsTsExpired(metadata.Expiry) {
			if !noLogs {
				log.Printf("Delete %s", filename)
			}
			fileBackend.Delete(context.Background(), filename)
		}
	}
}

func PeriodicCleanup(minutes time.Duration, filesDir string, metaDir string, noLogs bool) {
	c := time.Tick(minutes)
	for range c {
		Cleanup(filesDir, metaDir, noLogs)
	}

}
