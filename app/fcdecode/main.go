package main

import (
    "C"
    "fmt"
    "github.com/funcube-dev/go/fclib"
    "io"
    "log"
    "net"
    "os"
    "strconv"
    "strings"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/knadh/koanf"
    "github.com/knadh/koanf/parsers/toml"
    "github.com/knadh/koanf/providers/env"
    "github.com/knadh/koanf/providers/file"
    "github.com/knadh/koanf/providers/posflag"
    flag "github.com/spf13/pflag"
)

var config = readConfiguration()
var dataChan = make(chan []byte, 64)

var stats = struct {
    Decoded uint
}{}

// OnDataReady is called back when decoded data is ready for collection
func OnDataReady() {
    fmt.Println("Data ready for collect")
    decodedSize := uint32(256)
    var decodedFreq float32
    var decodedErrors int
    decodedSize = 256
    decoded := make([]byte, decodedSize)
    fclib.Decode_CollectLastData(&decoded[0], &decodedSize, &decodedFreq, &decodedErrors)

    fmt.Printf("Decoded Frequency: %.2fHz  Error Count: %d  data: % x", decodedFreq, decodedErrors, decoded)
    dataChan <- decoded
    // send zero length buffer to drop connection
    dataChan <- make([]byte, 0)
}

func main() {
    log.Printf("Using Config:\n%s\n", config.Sprint())
        
	ver := fclib.Library_GetVersion()
    log.Printf("Got audioLib version %d\n", ver)
    time.Sleep(time.Millisecond*50)
        
    if fclib.Dongle_Initialize() != 1 || fclib.Dongle_Exists() != 1 {
        log.Fatalf("Failed to initialise FUNcube Dongle\n" +
                   "* Check the Dongle is plugged in, maybe try a powered usb hub?\n" +
                   "* Also ensure the docker container is starting with the --privileged flag, as device access is required")
    }
    time.Sleep(time.Millisecond*50)
    log.Printf("Found and Initialised FUNcube Dongle.")
    
    if result := fclib.Decode_Initialize(); result != 1 {
        log.Fatalf("Failed to initialise Decode workers, result:%d", result)
    }    
    time.Sleep(time.Millisecond*50)
    log.Printf("Initialised Decode workers.")

    fclib.Callback_SetOnDecodeReady(OnDataReady)
    time.Sleep(time.Millisecond*50)
    log.Printf("Set decode callback function. ")

    freq := uint32(config.Float64("frequency"))
    if result := fclib.Dongle_SetFrequency(freq); result != 1 {
        log.Fatalf("Failed to set FUNcube Dongle frequency:%dHz, result:%d", freq, result)
    }
    time.Sleep(time.Millisecond*50)
    log.Printf("Set FUNcube Dongle frequency:%dHz", freq)

    enablebiasT := config.Bool("biast")
    if enablebiasT {
        fclib.Dongle_BiasTEnable(1)
        log.Print("Set FUNcube Dongle 5V Bias-T ON")
    } else {
        fclib.Dongle_BiasTEnable(0)
        log.Print("Set FUNcube Dongle 5V Bias-T OFF")
    }
        
    time.Sleep(time.Millisecond*50)
    
    audioIn := config.String("audiodevicein")
    audioOut := config.String("audiodeviceout")
    idAudioIn, errIn := strconv.Atoi(audioIn)
    idAudioOut, errOut := strconv.Atoi(audioOut)

    // if both numbers assume devce id's
    log.Println("*** Starting decode workers (may produce a few ALSA errors, just ignore!) ***")
    if nil != errIn || nil != errOut {
        // crashes everytime, needs some debugging...
        //fclib.Decode_Start(audioIn,audioOut,1,1)
        log.Fatalf("Sorry device names is an unimplemented feature, please use numeric device id's, or -1 for the defaults")
    }

    workers := uint32(config.Int("numdecoders"))
    if result := fclib.Decode_SetWorkerCount(workers); result != 1 {
        log.Fatalf("Failed to set number of decode workers, requested: %d workers, result:%d", workers, result)
    }

    if result := fclib.Decode_StartByIndex(idAudioIn,idAudioOut,1,1); result != 1 {
        log.Fatalf("*** Failed start decode workers, result:%d ***", result)
    }
    
    log.Println("*** Started decode workers, waiting for packet decodes ***")

    go sendData()

    serveStats()
}

func sendData() {
    log.Println("Ready to Send...")
    
    var err error
    var conn net.Conn
    var dst io.Writer
    var data []byte
    byteCount := 0
    
    for {
        // don't get more if we already have unsent
        if byteCount==0 {
            select {
            case data = <-dataChan:
                byteCount = len(data)
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
            host := config.String("connectaddress")
            port := config.String("connectport")
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
        
        written, err := dst.Write(data)
        if err!=nil {
            log.Println("Failed to write", err)
            conn.Close()
            dst=nil
            continue
        }        
        data = data[written:]
        byteCount-=written
        if(byteCount<0) {
            log.Println("Negative byte count???? resetting:", byteCount, written)
            byteCount = 0
        }
        fmt.Printf(">")        
    }
}

type Response struct {
    Data interface{} `json:"data,omitempty"`
}

func serveStats() {
    host := config.String("bindaddress")
    port := config.String("commandport")
    hostport := net.JoinHostPort(host, port)
    log.Println("Opening command listen socket...")

    r := gin.New()
//    r.Use(gin.Logger())
//    r.Use(gin.Recovery())

    apiv1 := r.Group("/api/v1")
    apiv1.Use()
    {
        apiv1.GET("/stats", func(c *gin.Context) {
            c.JSON(200, Response{
                Data: stats,
            })
        })
    }
    r.Run(hostport)
}

func readConfiguration() *koanf.Koanf {
	var konf = koanf.New(".")

	konf.Load(env.Provider("DEC_", ".", func(s string) string {
		return strings.Replace(strings.ToLower(
			strings.TrimPrefix(s, "DEC_")), "_", ".", -1)
	}), nil)

	for _, fileName := range []string{"/config/fcdecode.conf", "./fcdecode.conf"} {
		if _, err := os.Stat(fileName); err == nil {
			if err := konf.Load(file.Provider(fileName), toml.Parser()); err != nil {
				log.Fatalf("error loading config: %v", err)
			}
		}
	}

    flag.Float64("frequency", 145860000.0, "Frequency to tune FCD at")
    flag.Float64Slice("exclude", []float64 {}, "Frequency to exclude from tuning (guard band approx 100Hz either side of specifed Freq)")
    flag.Int("numdecoders", 5, "Number of simultaneous decoders (1-16)")
    flag.Bool("biast", false, "Enable 5V Bias-T output of FCD, true=On, false=Off")
    flag.String("audiodevicein", "-1", "Audio in device name or id (-1 use default)")
    flag.String("audiodeviceout", "-1", "Audio out device name or id (-1 use default)")
    flag.String("connectaddress", "encodeserver", "Address to connect to for sending encoded audio for transmission")
    flag.Int("connectport", int(0xFC02), "Port to connect to for sending decoded data (256 bytes chunks)")
    flag.String("bindaddress", "0.0.0.0", "Address to bind for TCP listen sockets")	
    flag.Int("commandport", int(0xFC01), "Port for incomming commands")
    flag.String("outdir", "", "Path in which to create funcubebin file")    
	flag.Parse()

	if err := konf.Load(posflag.Provider(flag.CommandLine, ".", konf), nil); err != nil {
		log.Fatalf("error loading config: %v", err)
	}

	return konf
}
