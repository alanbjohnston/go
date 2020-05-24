/* funcubelibwrap.i */
%feature("autodoc", "0");
/* turn on director wrapping for CallbackShim class */
%feature("director") CallbackShim;

%module(directors="1") funcubelibwrap
%{
#include "funcubelib/wintypes.h"
#include "funcubelib/funcubeLib.h"
#include "callbackshim.h"
%}

%insert(cgo_comment_typedefs) %{
#cgo LDFLAGS: -l:libfuncube.a -lstdc++ -lpthread -lm -lusb-1.0 -lfftw3f -lportaudio
%}

%typemap(gotype) uint32_t, const uint32_t & "uint32"
%typemap(gotype) int32_t, const int32_t & "int32"
%typemap(in) uint32_t, int32_t
%{ $1 = ($1_ltype)$input; %}

%typemap(in) const uint32_t &, const int32_t &
%{ $1 = ($1_ltype)&$input; %}

// apply the standard typemaps to other declarations:
// %apply "name type with standard typemap" {"name of type to map them to"}
%apply unsigned char {uint8_t};
%apply long long {int64_t};
%apply unsigned long long {uint64_t};

%ignore CCriticalSection;
%ignore CEvent;
%ignore CWorkerThread;
%ignore CPeakDetectThread;
%ignore IWorkerThreadClient;
%ignore CEncodeThread;
%ignore COMPLEXSTRUCT;
%ignore Caller::call;

%include "wintypes.h"
%include "funcubeLib.h"
%include "callbackshim.h"
