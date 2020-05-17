# All the go code used in the FUNcube project
app/fcdecode:
- decodes FUNcube formated (AO40) satellite transimissions into 256 byte frames, tracks peaks, tunes an FC dongle.

app/fcwarehouse:
- uploads FUNcube frames to the data warehouse.

app/fcencode:
- encodes 256 byte chunks of data into dbpsk format (with forward error correction) ready for transmission.

app/limetx:
- takes dbpsk encoded data and transmits it using a limesdr.

fcio utilities:
- TimedConn which wraps a connection to give a connection with read/write timeouts
- ReadSeekCloser wraps a ReadCloser to provide seeking if availabile on the underlying reader

fclib:
- go wrapper around the FUNcubeLib C/C++ library
