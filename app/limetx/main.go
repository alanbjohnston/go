package main

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/brian-armstrong/gpio"
	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/myriadrf/limedrv"
	flag "github.com/spf13/pflag"
)

type source struct {
	io.Reader
	io.Closer
	reader io.ReadCloser
	conn   *net.Conn
	file   *os.File
}

func (s source) Read(p []byte) (int, error) {
	if nil != s.conn {
		(*s.conn).SetReadDeadline(time.Now().Add(time.Second))
	}
	n, err := s.reader.Read(p)
	if nil != s.conn {
		(*s.conn).SetReadDeadline(time.Time{})
	}
	return n, err
}

func (s source) Close() error {
	if nil != s.conn {
		return (*s.conn).Close()
	}
	if nil != s.file {
		return (*s.file).Close()
	}
	return nil
}

func (s source) IsTimeout(err error) bool {
	neterr, ok := err.(net.Error)
	return ok && neterr.Timeout()
}

var config = readConfiguration()
var readerQueue = list.New()
var transmitChan = make(chan []float32, 8)
var lime *limedrv.LMSDevice
var txGpioPin *gpio.Pin

func main() {
	log.Printf("Using Config:\n%s\n", config.Sprint())

	gpioPinID := config.Int("gpio")
	if gpioPinID > 0 {
		pin := gpio.NewOutput(uint(gpioPinID), false)
		txGpioPin = &pin
		defer txGpioPin.Cleanup() // odd but works, defer is function, not block scoped
	}

	devices := limedrv.GetDevices()
	if len(devices) == 0 {
		log.Fatalln("No lime device found, restart container if device added after start or check udev rules and that usbdev package is installed")
	}

	d := devices[0]
	if len(devices) > 1 {
		log.Printf("Found %d limes, using: %v", len(devices), d)
	}

	log.Printf("Opening %s\n", d.DeviceName)
	lime = limedrv.Open(d) // Open the selected device
	defer lime.Close()     // Defer the close of the device

	log.Printf("Temp:%f/n", lime.GetTemperature())
	log.Println(lime.String())

	fileName := config.String("file")
	if len(fileName) > 0 {
		txfile, err := os.Open(fileName)
		if err != nil {
			log.Fatalf("Failed to open file %s error:%v", fileName, err)
		}
		readerQueue.PushBack(source{reader: txfile, file: txfile})
		transmitStart()
	}

	go fillTransmitChannel()
	go listen()
	commandListen()
}

func listen() {
	host := config.String("bindaddress")
	port := config.String("sampleport")
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

func transmitStart() {
	txch := lime.TXChannels[config.Int("channel")] // limedrv.ChannelA by default

	sampleRate := config.Float64("rate")
	oversample := config.Int("oversample")
	frequency := config.Float64("frequency")
	antennaName := config.String("antenna")
	txGain := config.Float64("gain")
	lpf := config.Float64("lpf")
	calibrationDelay := time.Duration(config.Float64("calibrationdelay")) * time.Second

	if txGpioPin != nil {
		log.Printf("Set gpio Low...")
		txGpioPin.Low()
	}

	log.Printf("Set Sample rate:%f oversample:%d", sampleRate, oversample)
	lime.SetSampleRate(sampleRate, oversample)

	// Set lpf starts calibration, so delay before starting tx
	txch.Enable().
		SetAntennaByName(antennaName). // limedrv.BAND2 by default
		SetGainNormalized(txGain).
		SetCenterFrequency(frequency).
		SetLPF(lpf)

	log.Println("Starting Calibration Delay:", calibrationDelay)
	time.Sleep(calibrationDelay)
	log.Println("Stopping Calibration Delay:", calibrationDelay)

	if txGpioPin != nil {
		log.Printf("Set gpio High...")
		txGpioPin.High()
	}

	log.Printf("Setting callback...")
	lime.SetTXCallback(realSampleCallback)
	log.Printf("Starting...")
	lime.Start()
}

func transmitStop() {
	txch := lime.TXChannels[config.Int("channel")] // limedrv.ChannelA by default

	if txGpioPin != nil {
		log.Printf("Set gpio Low...")
		txGpioPin.Low()
	}

	lime.Stop()
	txch.Disable()
	log.Println("Stopped Transmit")
}

func handleConnection(c net.Conn) {
	log.Printf("Connection from: %v", c)
	readerQueue.PushBack(source{reader: c, conn: &c})
}

func handleCommandConnection(c net.Conn) {
	log.Printf("Connection from: %v", c)
	defer c.Close()
}

func fillTransmitChannel() {
	loopFile := config.Bool("loopfile")
	idleSeconds := 0
	idleTimeout := config.Int("idletimeout")
	for {
		// if there's nothing to read from wait then try again
		if readerQueue.Len() == 0 {
			time.Sleep(time.Second)
			fmt.Printf(".")
			if lime.IsRunning() {
				if idleSeconds++; idleSeconds > idleTimeout {
					transmitStop()
				}
			}
			continue
		}
		if !lime.IsRunning() {
			transmitStart()
		}
		idleSeconds = 0

		src := readerQueue.Front().Value.(source)

		raw := make([]byte, 4096)
		bytesRead, err := src.Read(raw)
		if err == io.EOF || src.IsTimeout(err) {
			if src.file != nil {
				if !loopFile {
					fmt.Printf("|")
					lastRead := bytesRead
					src.file.Seek(0, 0)
					bytesRead, err = src.reader.Read(raw[lastRead:])
					bytesRead += lastRead
				} else {
					fmt.Printf("^")
					readerQueue.Remove(readerQueue.Front())
					(*src.file).Close()
					err = nil
				}
			}
			if src.conn != nil {
				fmt.Printf("^")
				readerQueue.Remove(readerQueue.Front())
				(*src.conn).Close()
				err = nil
			}
		}
		if err != nil {
			panic(err)
		}

		if bytesRead == 0 {
			continue
		}

		reader := bytes.NewReader(raw[:bytesRead])
		// create sample buffer based on data read
		samples := make([]float32, int(bytesRead/4))

		// fill sample buffer
		for i := 0; i < len(samples); i++ {
			err = binary.Read(reader, binary.LittleEndian, &samples[i])
			if err != nil {
				panic(err)
			}
		}
		fmt.Printf(">")
		transmitChan <- samples
	}
}

func realSampleCallback(data []complex64, channel int) int64 {
	var samples []float32
	sampleCount := 0
	select {
	case samples = <-transmitChan:
		sampleCount = len(samples)
	default:
		time.Sleep(time.Millisecond * 50)
		sampleCount = 0
	}

	if sampleCount == 0 {
		//pdata = (pdata)[:0]
		return 0
	}
	fmt.Printf("<")

	// resize output buffer based on data read
	//data = (data)[:sampleCount]
	//data := pdata

	// fill output buffer
	for i := 0; i < sampleCount; i++ {
		data[i] = complex64(complex(samples[i], 0.0))
	}
	return int64(sampleCount)}

func readConfiguration() *koanf.Koanf {
	var konf = koanf.New(".")

	konf.Load(env.Provider("LTX_", ".", func(s string) string {
		return strings.Replace(strings.ToLower(
			strings.TrimPrefix(s, "LTX_")), "_", ".", -1)
	}), nil)

	for _, fileName := range []string{"/config/limetx.conf", "./limetx.conf"} {
		if _, err := os.Stat(fileName); err == nil {
			if err := konf.Load(file.Provider(fileName), toml.Parser()); err != nil {
				log.Fatalf("error loading config: %v", err)
			}
		}
	}

	flag.Float64P("frequency", "f", float64(145.893e6), "Transmit frequency in Hz")
	flag.Float64("rate", float64(48000.0), "Audio sample rate Hz")
	flag.Int("oversample", int(32), "Oversampling rate [1,2,4,8,16,32], when multiplied by the sample rate must be within Lime limits")
	flag.StringP("antenna", "a", limedrv.BAND2, "Name of lime transmit antenna")
	flag.Int("channel", limedrv.ChannelA, "Name of lime transmit channel")
	flag.Float64("lpf", float64(5e6), "Bandwidth of lime analogue filter")
	flag.Float64P("gain", "g", float64(0.5), "Gain 0..1 (lime normalized gain)")
	flag.Float64P("calibrationdelay", "d", float64(5.0), "Delay before starting transmission to allow calibration to complete")
	flag.String("bindaddress", "0.0.0.0", "Address to bind for TCP listen sockets")
	flag.Int("sampleport", int(0xFC04), "Port for incomming samples")
	flag.Int("commandport", int(0xFC05), "Port for incomming commands")
	flag.String("file", "", "Path to dbpsk file to transmit (float32 LE 48000Khz)")
	flag.Bool("loopfile", false, "Send the file in an endless loop")
	flag.Int("idletimeout", 30, "Seconds of no data before stopping transmission")
	flag.Int("gpio", -1, "Raspberry PI GPIO pin to toggle, high when transmitting, low when idle (default -1 dont toggle")
	flag.Parse()

	if err := konf.Load(posflag.Provider(flag.CommandLine, ".", konf), nil); err != nil {
		log.Fatalf("error loading config: %v", err)
	}

	return konf
}
