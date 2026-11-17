package sources

import (
	"errors"
	"fmt"
	"net/http"
)

// StatusError is a non-2xx answer from a source.
type StatusError struct {
	Source string
	Code   int
}

func (e StatusError) Error() string { return fmt.Sprintf("%s %d", e.Source, e.Code) }

func isNotFound(err error) bool {
	var se StatusError
	return errors.As(err, &se) && se.Code == http.StatusNotFound
}
