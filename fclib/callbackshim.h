/* File : callbackshim.h */

#include <cstdio>
#include <iostream>
#include <functional>

class CallbackShim {
public:
	virtual ~CallbackShim() {}
	virtual void run() {};    
};
CallbackShim* g_shimInstance=0;

void OnDataReadyShim() {
    if (g_shimInstance) g_shimInstance->run(); 
}

void CleanupCallbackShim() { 
    CallbackShim* tmp=g_shimInstance;
    g_shimInstance = 0;
    delete tmp;
}

void InitialiseCallbackShim(CallbackShim *cb) {
        CleanupCallbackShim(); 
        g_shimInstance = cb;         
        Decode_SetOnDataReadyCallback(&OnDataReadyShim);
}

void RunCallback() { 
    OnDataReadyShim(); 
}
