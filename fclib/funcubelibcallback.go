package fclib

import (
	"sync"
)

//OnDecodeReadyFunc Signature of method required to receive notifications data ready
type OnDecodeReadyFunc func()

type goCallback struct {
	onDecodeReadyCallback OnDecodeReadyFunc
}

var callbackInstance *goCallback

//var callerInstance Caller
var once sync.Once

// Callback_SetOnDecodeReady sets the function to be called when data is decoded and ready to collect
func Callback_SetOnDecodeReady(callbackFunc OnDecodeReadyFunc) {
	once.Do(func() {
		callbackInstance = &goCallback{}
		InitialiseCallbackShim(NewDirectorCallbackShim(callbackInstance))
	})
	callbackInstance.onDecodeReadyCallback = callbackFunc
}

// Callback_ClearOnDecodeReady clears the function to be called when data is decoded and ready to collect
func Callback_ClearOnDecodeReady(callbackFunc OnDecodeReadyFunc) {
	callbackInstance.onDecodeReadyCallback = nil
}

func Callback_Test() {
	RunCallback()
}

//Run does stuff too
func (p *goCallback) Run() {
	if nil != p.onDecodeReadyCallback {
		p.onDecodeReadyCallback()
	}
}
