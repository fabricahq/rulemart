// Package jobs encodes and decodes the jobs the worker takes from its queue: update a vetted library, named by its
// code host and the host's repository ID, or check a listing, named by its ID. No job carries a URL, so a queued
// message can't point the worker at a repository nobody vetted or listed: the worker fetches only from what the
// catalog stored, or what the code host says the vetted ID or listed name is.
package jobs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
)

// Job is one job the worker runs: exactly one of Library and Listing is set.
type Job struct {
	// Library is the vetted library to update, or the zero key for a listing's job.
	Library domain.LibraryKey
	// Listing is the ID of the listing to check, or 0 for a library's job.
	Listing int64
}

// message is a job as its queue holds it: {"host":"github","repositoryID":"42"} or {"listing":7}.
type message struct {
	Host         string `json:"host,omitempty"`
	RepositoryID string `json:"repositoryID,omitempty"`
	Listing      int64  `json:"listing,omitempty"`
}

// Update returns the message for the job that updates the vetted library.
func Update(library domain.LibraryKey) string {
	return encode(message{Host: library.Host, RepositoryID: library.RepositoryID})
}

// CheckListing returns the message for the job that checks the listing id.
func CheckListing(id int64) string {
	return encode(message{Listing: id})
}

func encode(m message) string {
	body, err := json.Marshal(m)
	if err != nil {
		// A message holds two strings or a number, which always encode.
		panic(err)
	}
	return string(body)
}

// Parse returns the job body holds. It refuses anything but exactly one JSON object with known fields that names a
// library by host and repository ID, or a listing by a positive ID, and not both. It doesn't check that the library is
// vetted or the listing exists; the worker does.
func Parse(body string) (Job, error) {
	decoder := json.NewDecoder(bytes.NewReader([]byte(body)))
	decoder.DisallowUnknownFields()
	var m message
	if err := decoder.Decode(&m); err != nil {
		return Job{}, fmt.Errorf("decode job: %v", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return Job{}, errors.New("decode job: expected one JSON object and nothing after it")
	}
	library := m.Host != "" || m.RepositoryID != ""
	switch {
	case library && m.Listing != 0:
		return Job{}, errors.New("decode job: expected a library or a listing, not both")
	case library:
		return Job{Library: domain.LibraryKey{Host: m.Host, RepositoryID: m.RepositoryID}}, nil
	case m.Listing > 0:
		return Job{Listing: m.Listing}, nil
	}
	return Job{}, errors.New("decode job: expected a library's host and repository ID, or a listing's ID")
}
