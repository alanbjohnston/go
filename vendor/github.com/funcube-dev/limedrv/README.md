[![Build Status](https://api.travis-ci.org/racerxdl/limedrv.svg?branch=master)](https://travis-ci.org/racerxdl/limedrv) [![Apache License](https://img.shields.io/badge/license-Apache-blue.svg)](https://tldrlegal.com/license/apache-license-2.0-(apache-2.0)) [![Go Report](https://goreportcard.com/badge/github.com/myriadrf/limedrv)](https://goreportcard.com/report/github.com/myriadrf/limedrv)

# limedrv
LimeSuite Wrapper on Go (Driver for LimeSDR Devices)

# Usage

So far I need to do all the comments for the methods (since go auto-generates the documentation).
But while I do that, you can check the examples. The documentation is available at: [https://godoc.org/github.com/myriadrf/limedrv](https://godoc.org/github.com/myriadrf/limedrv)

# Examples

So far there is a functional WBFM Radio that uses SegDSP for demodulating. You can check it at `_examples/limefm`. To compile, just go to the folder and run:

```bash
go build
```

It will generate a `limefm` executable in the folder. It outputs the raw Float32 audio into stdout. For example, you can listen to the radio by using ffplay:

```bash
./limefm -antenna LNAL -centerFrequency 106300000 -channel 0 -gain 0.5 -outputRate 48000 | ffplay -f f32le -ar 48k -ac 1 -
```

There is also a FFT Generator in `fftaverage` folder. The parameters need to be set in the code, but it does generate a nice JPEG with the FFT.


# Static Linking

Since `libLimeSuite` doesn't generate static libraries by default (see https://github.com/myriadrf/LimeSuite/issues/241), you should manually compile it to provide the `libLimeSuite.a` thats needed for static linking.


You can just do the normal LimeSuite build with `-DBUILD_SHARED_LIBS=OFF` to statically build LimeSuite (that does not break current dynamic linked stuff)

```bash
# Assumes in libLimeSuite folder
cmake .. -DBUILD_SHARED_LIBS=OFF
make -j8
sudo make install
```

Then you can change the `limewrap.go` line with the linking definition from:

```go
#cgo LDFLAGS: -lLimeSuite
```

to

```go
#cgo LDFLAGS: -l:libLimeSuite.a -l:libstdc++.a -lm -lusb-1.0
```

And then compile normally your application. The libLimeSuite should be embedded inside your executable.