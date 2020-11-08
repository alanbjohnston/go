package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func makeFrame(seedBuf []byte, repeat int, retry int) *Frame {
	frame, _ := NewFrame(bytes.Repeat(seedBuf, repeat), retry)
	return frame
}

func repeatString(char string, repeat int) *string {
	var result string
	for i := 0; i < repeat; i++ {
		result += char
	}
	return &result
}

func TestFrame_NewFrame(t *testing.T) {
	_, actualError := NewFrame([]byte{0, 0, 0}, 0)

	assert.NotEqual(t, nil, actualError)
}

func TestFrame_CanRetry(t *testing.T) {
	tests := []struct {
		name     string		
		numRetries int
		canRetry []bool		
	}{
		{"no retry",        0, []bool{false, false, false, false, false}},
		{"one retry",       1, []bool{true,  false, false, false, false}},
		{"two retry",       2, []bool{true,  true,  false, false, false}},
		{"infinite retry", -1, []bool{true,  true,  true,  true,  true }},		
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := makeFrame([]byte{0}, 256, tt.numRetries)
			for i:=0;i<len(tt.canRetry);i++ {
				want := tt.canRetry[i]
				got := f.CanRetry()				
				if got != want {
					t.Errorf("iteration: %d, want:%t, got:%t, ", i, want, got)
					return
				}
				f.DecrementRetry()
			}
		})
	}
}

func TestFrame_GetWarehouseDigest(t *testing.T) {
	tests := []struct {
		name     string
		f        *Frame
		authCode string
		want     string
		wantErr  bool
	}{
		{"nil input", &Frame{[]byte{}, 0}, "aaaaaaaaaaaaaaaaaaa", "f6bd180a4c0ffb0aee7a4c89273edb3b", false},
		{"zero input", makeFrame([]byte{0}, 256, 0), "aaaaaaaaaaaaaaaaaaa", "4e74fba62f9a27fa11650f17c98d97af", false},
		{"255 input", makeFrame([]byte{255}, 256, 0), "aaaaaaaaaaaaaaaaaaa", "7f289e866f4035ef126d1d72b4cee122", false},
		{"mixed input", makeFrame([]byte{00, 255}, 128, 0), "aaaaaaaaaaaaaaaaaaa", "e317d85ab37b6746771069dadf271531", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.f.GetWarehouseDigest(tt.authCode)
			if (err != nil) != tt.wantErr {
				t.Errorf("Frame.ToWarehouse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Frame.ToWarehouse() = %v, want %v", got, tt.want)
			}
		})
	}
}
