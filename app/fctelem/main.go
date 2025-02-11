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
var sendDisabled = false

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

	fmt.Printf("Decoded Frequency: %.2fHz  Error Count: %d  data: % x\n", decodedFreq, decodedErrors, decoded)

	// bail if not sending
	if sendDisabled {
		fmt.Println("Discarded result, send disabled.")
		return
	}

	select {
	case dataChan <- decoded:
	default:
		fmt.Println("Discarded result channel full.")
	}

	// send zero length buffer to drop connection
	select {
	case dataChan <- make([]byte, 0):
	default:
		fmt.Println("Failed to send inter frame marker.")
	}
}

func main() {
	log.Printf("Using Config:\n%s\n", config.Sprint())

	if config.String("connectaddress") == "" && len(config.Strings("connectlocations"))==0 {
		log.Println("Empty connectaddress and connectlocations, disabled sending")
		sendDisabled = true
	}

	connectLocations := config.Strings("connectlocations")
	// append original connect address/port to location for backward compatibility
	if config.String("connectaddress") != "" {
		host := config.String("connectaddress")
		port := config.String("connectport")

		connectLocations = append(connectLocations, net.JoinHostPort(host, port))
	}

	ver := fclib.Library_GetVersion()
	log.Printf("Got audioLib version %d\n", ver)
	time.Sleep(time.Millisecond * 50)

	if fclib.Dongle_Initialize() != 1 || fclib.Dongle_Exists() != 1 {
		log.Fatalf("Failed to initialise FUNcube Dongle\n" +
			"* Check the Dongle is plugged in, maybe try a powered usb hub?\n" +
			"* Also ensure the docker container is starting with the --privileged flag, as device access is required")
	}
	time.Sleep(time.Millisecond * 50)
	log.Println("Found and Initialised FUNcube Dongle.")

	if result := fclib.Decode_Initialize(); result != 1 {
		log.Fatalf("Failed to initialise Decode workers, result:%d", result)
	}
	time.Sleep(time.Millisecond * 50)
	log.Println("Initialised Decode workers.")

	fclib.Callback_SetOnDecodeReady(OnDataReady)
	time.Sleep(time.Millisecond * 50)
	log.Println("Set decode callback function.")

	freq := uint32(config.Float64("frequency"))
	if result := fclib.Dongle_SetFrequency(freq); result != 1 {
		log.Fatalf("Failed to set FUNcube Dongle frequency:%dHz, result:%d", freq, result)
	}

	freqLow := freq - 100000
	freqHigh := freq + 100000
	if result := fclib.Decode_SetAutoTuneFrequencyRange(freqLow, freqHigh); result != 1 {
		log.Fatalf("Failed to set auto tune frequency range low: %dHz, high: %dHz, result:%d", freqLow, freqHigh, result)
	}

	time.Sleep(time.Millisecond * 50)
	log.Printf("Set FUNcube Dongle frequency:%dHz\n", freq)

	enablebiasT := config.Bool("biast")
	if enablebiasT {
		fclib.Dongle_BiasTEnable(1)
		log.Println("Set FUNcube Dongle 5V Bias-T ON")
	} else {
		fclib.Dongle_BiasTEnable(0)
		log.Println("Set FUNcube Dongle 5V Bias-T OFF")
	}

	time.Sleep(time.Millisecond * 50)

	audioIn := config.String("audiodevicein")
	audioOut := config.String("audiodeviceout")
	idAudioIn, errIn := strconv.Atoi(audioIn)
	idAudioOut, errOut := strconv.Atoi(audioOut)

	// if both numbers assume devce id's
	log.Println("*** Starting decode workers (may produce a few ALSA errors, just ignore!) ***")
	if nil != errIn || nil != errOut {
		// crashes every time, needs some debugging...
		//fclib.Decode_Start(audioIn,audioOut,1,1)
		log.Fatalf("Sorry device names is an unimplemented feature, please use numeric device id's, or -1 for the defaults")
	}

	workers := uint32(config.Int("numdecoders"))
	if result := fclib.Decode_SetWorkerCount(workers); result != 1 {
		log.Fatalf("Failed to set number of decode workers, requested: %d workers, result:%d", workers, result)
	}

	if result := fclib.Decode_StartByIndex(idAudioIn, idAudioOut, 1, 1); result != 1 {
		log.Fatalf("*** Failed start decode workers, result:%d ***", result)
	}

	log.Println("*** Started decode workers, waiting for packet decodes ***")

	var dataChans []chan []byte

	// start one sendData routine per destination host
	for _, loc := range connectLocations {
		ch := make(chan []byte, 64)
		dataChans = append(dataChans, ch)
		go sendData(ch, loc)
	}

	go cloneDataChannel(dataChan, dataChans)

	serveStats()
}


func cloneDataChannel(srcChan chan []byte, destChans []chan []byte) {
	var data []byte
	for {
		// take data off the source channel, block until there's something to read
		fmt.Print("v")
		data = <-srcChan
		fmt.Print("^")
		//send it to all the dest channels (dont block if channel full)
		for _, dest := range destChans {
			select {
			case dest <- data:
			default:
				fmt.Print("x")
				continue
			}
			fmt.Print("+")
		}
	}
}

func sendData(srcChan chan []byte, destLoc string) {
	log.Println("Starting send worker for:", destLoc)

	var err error
	var conn net.Conn
	var dst io.Writer
	var data []byte
	backoffSecs := 5
	byteCount := 0

	for {
		// no need to rush sleep between attempts
		time.Sleep(time.Millisecond * 250)

		// don't get more if we already have unsent
		if byteCount == 0 {
			select {
			case data = <-srcChan:
				byteCount = len(data)
				fmt.Printf("-")

				// a zero byte buffer on the channel is an inter frame marker
				// so drop the connection
				if byteCount == 0 {
					if err := conn.Close(); err != nil {
						log.Println("Frame complete, failed to close: ", err)
					}
					dst = nil
					fmt.Printf("|")
					continue
				}
			default:
				continue
			}
		}

		// don't connect if already connected
		if nil == dst {
			conn, err = net.DialTimeout("tcp", destLoc, time.Second*5)
			if err != nil {
				if backoffSecs += 5; backoffSecs > 120 {
					backoffSecs = 120
				}
				log.Printf("\nFailed to connect %v\nRetry in %d seconds", err, backoffSecs)
				time.Sleep(time.Second * time.Duration(backoffSecs))
				continue
			}

			if dst, _ = conn.(io.Writer); nil == dst {
				log.Println("Error getting writer...")
				continue
			}
		}

		written, err := dst.Write(data)
		if err != nil {
			log.Println("Failed to write", err)
			if err := conn.Close(); err != nil {
				log.Println("Failed to close: ", err)
			}
			dst = nil
			continue
		}
		data = data[written:]
		byteCount -= written
		if byteCount < 0 {
			log.Println("Negative byte count???? resetting:", byteCount, written)
			byteCount = 0
		}
		backoffSecs = 5
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
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	apiv1 := r.Group("/api/v1")
	apiv1.Use()
	{
		apiv1.GET("/stats", func(c *gin.Context) {
			c.JSON(200, Response{
				Data: stats,
			})
		})
	}
	_ = r.Run(hostport)
}

func readConfiguration() *koanf.Koanf {
	var konf = koanf.New(".")

	err := konf.Load(env.Provider("DEC_", ".", func(s string) string {
		return strings.Replace(strings.ToLower(
			strings.TrimPrefix(s, "DEC_")), "_", ".", -1)
	}), nil)
	if err != nil {
		log.Printf("error reading environment variables: %v", err)
	}

	for _, fileName := range []string{"/config/fcdecode.conf", "./fcdecode.conf"} {
		if _, err := os.Stat(fileName); err == nil {
			if err := konf.Load(file.Provider(fileName), toml.Parser()); err != nil {
				log.Fatalf("error loading config: %v", err)
			}
		}
	}

	flag.Float64("frequency", 145860000.0, "Frequency to tune FCD at")
	flag.Float64Slice("exclude", []float64{}, "Frequency to exclude from tuning (guard band approx 100Hz either side of specifed Freq)")
	flag.Int("numdecoders", 5, "Number of simultaneous decoders (1-16)")
	flag.Bool("biast", false, "Enable 5V Bias-T output of FCD, true=On, false=Off")
	flag.String("audiodevicein", "-1", "Audio in device name or id (-1 use default)")
	flag.String("audiodeviceout", "-1", "Audio out device name or id (-1 use default)")
	flag.String("connectaddress", "encodeserver", "Address to connect to for sending decoded data for uploading or encoding, empty string disables data send")
	flag.Int("connectport", int(0xFC02), "Port to connect to for sending decoded data (256 bytes chunks)")	
	flag.String("bindaddress", "0.0.0.0", "Address to bind for TCP listen sockets")
	flag.StringSlice("connectlocations", []string{}, "Address:Port combination to connect to for sending decoded data, multiple locations can be specified in the format [\"host1:port1\", \"host2:port2\"] the data will be copied to all")
	flag.Int("commandport", int(0xFC01), "Port for incoming commands")
	flag.String("outdir", "", "Path in which to create funcubebin files")
	flag.Parse()

	if err := konf.Load(posflag.Provider(flag.CommandLine, ".", konf), nil); err != nil {
		log.Fatalf("error loading config: %v", err)
	}


	return konf
}
