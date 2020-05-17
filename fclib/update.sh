CURRENT_DIR=$(dirname $(readlink -f $0))
echo Runing from: $CURRENT_DIR
cd $CURRENT_DIR

rm funcubelibwrap.go
rm funcubelibwrap_wrap.cxx
rm funcubelibwrap_wrap.h
swig -cgo -go -c++ -intgosize 64 -package fclib -I/usr/local/include/funcubelib funcubelibwrap.i
