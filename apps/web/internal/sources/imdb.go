package sources

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"filmarsivi/web/internal/domain"
)

// IMDbRatingsURL is IMDb's official dataset for personal, non-commercial use
// (https://developer.imdb.com/non-commercial-datasets/). Refreshed daily by IMDb.
const IMDbRatingsURL = "https://datasets.imdbws.com/title.ratings.tsv.gz"

// ParseIMDbRatings reads title.ratings.tsv (already decompressed): a header line, then
// `tconst \t averageRating \t numVotes`. Malformed rows are skipped.
func ParseIMDbRatings(r io.Reader, add func(domain.IMDbRating) error) (int, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	n := 0
	first := true
	for sc.Scan() {
		line := sc.Text()
		if first {
			first = false
			if strings.HasPrefix(line, "tconst") {
				continue
			}
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 3 || !strings.HasPrefix(parts[0], "tt") {
			continue
		}
		rating, err1 := strconv.ParseFloat(parts[1], 64)
		votes, err2 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil || rating <= 0 || votes < 0 {
			continue
		}
		if err := add(domain.IMDbRating{TConst: parts[0], Rating: rating, Votes: votes}); err != nil {
			return n, err
		}
		n++
	}
	return n, sc.Err()
}

// IMDbSink stores a full replacement of the IMDb ratings. fill is called inside one
// transaction; if it fails, the old data must stay.
type IMDbSink interface {
	ReplaceIMDbRatings(ctx context.Context, fill func(add func(domain.IMDbRating) error) error) error
}

// SyncIMDbRatings downloads the gzipped dataset and replaces the stored copy.
func SyncIMDbRatings(ctx context.Context, client *http.Client, url string, sink IMDbSink) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	res, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 0, StatusError{Source: "IMDb", Code: res.StatusCode}
	}
	gz, err := gzip.NewReader(res.Body)
	if err != nil {
		return 0, fmt.Errorf("IMDb gzip: %w", err)
	}
	defer gz.Close()
	var n int
	err = sink.ReplaceIMDbRatings(ctx, func(add func(domain.IMDbRating) error) error {
		var perr error
		n, perr = ParseIMDbRatings(gz, add)
		if perr == nil && n == 0 {
			return fmt.Errorf("IMDb: empty dataset")
		}
		return perr
	})
	return n, err
}
