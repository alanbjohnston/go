package fcio 

import (
	"errors"
	"io"
)

// ReadSeekCloser Provides ReadCloser interface for os.File or net.Conn readers
type ReadSeekCloser struct {
	io.Reader
	io.Closer
	io.Seeker
	reader io.ReadCloser
	seeker io.Seeker
}

// NewReadSeekCloser create a ReadSeekCloser, can read close and seek/test if seeking possible
func NewReadSeekCloser(reader io.ReadCloser) (*ReadSeekCloser, error) {
	if reader == nil {
		return nil, errors.New("invalid reader (io.ReadCloser) parameter")
	}
	// grab the seeker interface just the once here
	seeker, _ := reader.(io.Seeker)
	return &ReadSeekCloser{
		reader: reader,
		seeker: seeker,
	}, nil
}

// Read returns data from the contained reader
func (rsc ReadSeekCloser) Read(p []byte) (int, error) {
	return rsc.reader.Read(p)
}

// CanSeek does the contained reader support seeking
func (rsc ReadSeekCloser) CanSeek() bool {
	return rsc.seeker != nil
}

// Seek sets the offset for the next Read
func (rsc ReadSeekCloser) Seek(offset int64, whence int) (int64, error) {
	if nil == rsc.seeker {
		return 0, errors.New("unsupported operation, no seek interface on contained ReadCloser")
	}
	return rsc.seeker.Seek(offset, whence)
}

// Close closes the contained reader
func (rsc ReadSeekCloser) Close() error {
	return rsc.reader.Close()
}
