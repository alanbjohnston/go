package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io"
)

const frameSize int = 256

// Frame container for a complete data frame
type Frame struct {
    data []byte
    retryCount int
}

// NewFrame creates an instance of a frame object
func NewFrame(buf []byte, maxRetry int) (*Frame, error) {
	if buf == nil || len(buf)!=frameSize {
        return nil, errors.New("Missing or incorrect size input buffer for Frame")
	}
	
	frameData := make([]byte, frameSize)
	copy(frameData, buf)
	return &Frame{frameData, maxRetry},nil
}

// GetWarehouseDigest gets the md5 digest for submitting to warehouse
func (f Frame) GetWarehouseDigest(authCode string) (string, error) {
	hexData := hex.EncodeToString(f.data)
	digestInput := []byte(hexData + ":" + authCode)
	hash := md5.Sum(digestInput)
	
	hexHash := hex.EncodeToString( hash[0:] )

	return hexHash, nil
}

func (f Frame) GetWarehousePayload() io.Reader {
	// odd format, but seems to be utf8 byte stream of hex encoded data?
	hexData := hex.EncodeToString(f.data)
	payloadBytes := []byte("data=" + hexData)
	return bytes.NewReader(payloadBytes)
}

func (f Frame) CanRetry() bool {
	return f.retryCount>0 || f.retryCount == -1
}

func (f Frame) RemainingRetry() int {
	return f.retryCount
}

func (f *Frame) DecrementRetry() {
	if f.retryCount > 0 {
		f.retryCount--
	}
}