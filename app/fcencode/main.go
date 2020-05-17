package main

import (
    "C"
    "container/list"
    "fmt"
    fc "funcube.org.uk/internal"
    alib "funcube.org.uk/internal/audiolibwrap"
    "io"
    "log"
    "net"
    "os"
    "strings"
    "time"

    "github.com/knadh/koanf"
    "github.com/knadh/koanf/parsers/toml"
    "github.com/knadh/koanf/providers/env"
    "github.com/knadh/koanf/providers/file"
    "github.com/knadh/koanf/providers/posflag"
    flag "github.com/spf13/pflag"
)

var config = readConfiguration()
var readerQueue = list.New()
var dataChan = make(chan []byte, 64)
var bpskChan = make(chan []byte, 64)

func main() {
    log.Printf("Using Config:\n%s\n", config.Sprint())

	devices := alib.Library_GetVersion()
    log.Printf("Lib version %d\n", devices)
    
    fileName := config.String("file")
    if len(fileName) > 0 {
	    fcbinfile, err := os.Open(fileName)
	    if err != nil {
            log.Fatalf("Failed to open file %s error:%v", fileName, err)
        }
        reader, err := fc.NewReadSeekCloser(fcbinfile)
        if err != nil {
            log.Fatalf("Failed to get reader from file %s error:%v", fileName, err)
        }
        readerQueue.PushBack(reader)
	}

	log.Printf("Done\n")

    go readData()
    go encodeData()
    go sendData()
    go listen()
    commandListen()
}

func readData() {
    loopFile := config.Bool("loopfile")
    for {
        // if there's nothing to read from wait then try again
        if readerQueue.Len() == 0 {
            time.Sleep(time.Second)
            fmt.Printf(".")
            continue
        }
        src := readerQueue.Front().Value.(*fc.ReadSeekCloser)
                
        raw := make([]byte, 256)
        bytesRead, err := src.Read(raw)
        if err == io.EOF {
            if loopFile && src.CanSeek() {
                fmt.Printf("|")
                lastRead := bytesRead
                src.Seek(0, 0)
                bytesRead, err = src.Read(raw[lastRead:])
                bytesRead += lastRead
            } else {            
                fmt.Printf("^")
                readerQueue.Remove(readerQueue.Front())
                src.Close()
                err = nil
            }
        }
        if err != nil {
            panic(err)
        }
        
        if bytesRead != 256 {
            continue
        }

        fmt.Printf("<")
        dataChan <- raw 
    }
}

func encodeData() {
    init := alib.Encode_Initialize()
    log.Printf("Initialised %d\n", init)
    
    var raw []byte
    for {
        byteCount := 0
        select {
        case raw = <-dataChan:
            byteCount = len(raw)
            fmt.Printf("!")
        default:
            time.Sleep(time.Millisecond*50)
            continue
        }

        if byteCount != 256 {            
            log.Printf("Warning: Got %d bytes, need 256... skipping\n", byteCount)
            continue
        }

        alib.Encode_PushData(&raw[0], 256)
        
        var bpskSize uint32
        for alib.Encode_AllDataCollected() == 0 {                        
            bpskSize = 0
            if alib.Encode_CanCollect() > 0 {
                bpskSize = 1280 // (40*8*4)
                bpskBuffer := make([]byte, bpskSize)
                
                alib.Encode_CollectSamples(&bpskBuffer[0], &bpskSize)
                // dont send zero length buffers (shouldn't ever happen)
                if bpskSize==0 {
                    fmt.Printf("0")
                    continue
                }

                fmt.Printf("~")
                bpskChan <- bpskBuffer[:bpskSize]                
            }
            // pause a bit if there was nothing to collect or we collected nothing!
            if bpskSize == 0 {
                time.Sleep(15 * time.Millisecond)
            }
        }

        // send zero length buffer to drop connection
        bpskChan <- make([]byte, 0)
    }
}

func sendData() {
    log.Println("Ready to Send...")
    
    var err error
    var conn net.Conn
    var dst io.Writer
    var bpsk []byte
    byteCount := 0
    
    for {
        // don't get more if we already have samples
        if byteCount==0 {
            select {
            case bpsk = <-bpskChan:
                byteCount = len(bpsk)
                fmt.Printf("-")
                // a zero byte buffer on the channel is an inter frame marker
                // so drop the connection
                if byteCount == 0 {
                    conn.Close()
                    dst=nil
                    fmt.Printf("|")
                    continue
                }
            default:
                time.Sleep(time.Millisecond*50)
                continue
            }
        }

        // don't connect if already connected
        if nil == dst {
            host := config.String("limetxserver")
            port := config.String("limetxport")
            hostport := net.JoinHostPort(host, port)            
            conn, err = net.DialTimeout("tcp",hostport, time.Second*5)
            if err != nil {
                log.Println("Failed to connect", err)
                time.Sleep(time.Second*5)
                continue
            }
            
            if dst, _ = conn.(io.Writer); nil==dst {
                log.Println("Error getting writer...")
                continue
            }
        }
        
        written, err := dst.Write(bpsk)
        if err!=nil {
            log.Println("Failed to write", err)
            conn.Close()
            dst=nil
            continue
        }        
        bpsk = bpsk[written:]
        byteCount-=written
        if byteCount<0 {
            log.Println("Negative byte count???? resetting:", byteCount, written)
            byteCount = 0
        }
        fmt.Printf(">")        
    }
}

func listen() {
    host := config.String("bindaddress")
    port := config.String("dataport")
    hostport := net.JoinHostPort(host, port)
    log.Println("Opening listen socket...")
    lsock, err := net.Listen("tcp4", hostport)    
    if err != nil {
            fmt.Println(err)
            return
    }
    defer lsock.Close()
    log.Printf("Listening on socket: %s", hostport)
    
    for {
        c, err := lsock.Accept()
        if err != nil {
                fmt.Println(err)
                return
        }
        handleConnection(c)        
    }
}

func commandListen() {
    host := config.String("bindaddress")
    port := config.String("commandport")
    hostport := net.JoinHostPort(host, port)
    log.Println("Opening command listen socket...")
    lsock, err := net.Listen("tcp4", hostport)    
    if err != nil {
            fmt.Println(err)
            return
    }
    defer lsock.Close()
    log.Printf("Listening for commands on socket: %s", hostport)
    
    for {
        c, err := lsock.Accept()
        if err != nil {
                fmt.Println(err)
                return
        }
        handleCommandConnection(c)        
    }
}

func handleConnection(c net.Conn) {
    log.Printf("Connection from: %v", c)

    tc, err := fc.NewTimedConn(c, time.Second)
    if err != nil {
        log.Printf("Failed to create TimedConn, ignoring error:%v", err)
        return
    }
    reader, err := fc.NewReadSeekCloser(tc)
    if err != nil {
        log.Printf("Failed to create reader, ignoring error:%v", err)
        return
    }
    readerQueue.PushBack(reader)
}

func handleCommandConnection(c net.Conn) {
    log.Printf("Connection from: %v", c)
}

func readConfiguration() *koanf.Koanf {
	var konf = koanf.New(".")

	konf.Load(env.Provider("ENC_", ".", func(s string) string {
		return strings.Replace(strings.ToLower(
			strings.TrimPrefix(s, "ENC_")), "_", ".", -1)
	}), nil)

	for _, fileName := range []string{"/config/fcencode.conf", "./fcencode.conf"} {
		if _, err := os.Stat(fileName); err == nil {
			if err := konf.Load(file.Provider(fileName), toml.Parser()); err != nil {
				log.Fatalf("error loading config: %v", err)
			}
		}
	}

    flag.Float64("rate", float64(48000.0), "Audio sample rate Hz (for now, must be 48kHz)")
    flag.Int("idletimeout", 3, "Seconds of no data to send before dropping connection to limetxserver")
    flag.String("limetxserver", "limeserver", "Address to connect to for sending encoded audio for transmission")
    flag.Int("limetxport", int(0xFC04), "Port to connect to for sending encoded audio for transmission")
    flag.String("bindaddress", "0.0.0.0", "Address to bind for TCP listen sockets")
	flag.Int("dataport", int(0xFC02), "Port for incomming decoded data (256 bytes chunks)")
    flag.Int("commandport", int(0xFC03), "Port for incomming commands")
    flag.String("file", "", "Path to funcubebin file to encode (multiple of 256 bytes in length)")
    flag.Bool("loopfile", false, "Send the file in an endless loop")
	flag.Parse()

	if err := konf.Load(posflag.Provider(flag.CommandLine, ".", konf), nil); err != nil {
		log.Fatalf("error loading config: %v", err)
	}

	return konf
}
