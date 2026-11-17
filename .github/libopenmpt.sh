#!/bin/sh
# Builds libopenmpt's shared library from its official source, pinned by version and SHA-256, into the directory $1,
# under the name openlemm looks for beside its executable (libPaths in cmd/openlemm/sound.go), with libopenmpt's licence.
# openlemm plays only ProTracker modules, so every optional dependency is off. It runs on each release target's own
# runner: macOS, Linux, or Windows in an MSYS2 shell (UCRT64 on amd64, CLANGARM64 on arm64).
set -eu
VERSION=0.8.10
SHA256=95073a10d33d0ed4352f41b68edccb9636002d2626f9fd2e00b191fcaabdd876

out=$1
tmp=$(mktemp -d)
mkdir -p "$out"
src="libopenmpt-$VERSION+release"
curl -fsSL -o "$tmp/src.tar.gz" "https://lib.openmpt.org/files/libopenmpt/src/$src.makefile.tar.gz"
echo "$SHA256  $tmp/src.tar.gz" | sha256sum -c -
tar -xzf "$tmp/src.tar.gz" -C "$tmp"
cd "$tmp/$src"

set -- SHARED_LIB=1 STATIC_LIB=0 SHARED_SONAME=0 EXAMPLES=0 OPENMPT123=0 TEST=0 \
	NO_ZLIB=1 NO_MPG123=1 NO_OGG=1 NO_VORBIS=1 NO_VORBISFILE=1 NO_MINIZ=1 NO_MINIMP3=1 NO_STBVORBIS=1
case $(uname -s) in
Darwin)
	# Go 1.27's oldest supported macOS.
	make -j4 "$@" MACOSX_VERSION_MIN=12.0
	cp bin/libopenmpt.so "$out/libopenmpt.dylib"
	;;
Linux)
	make -j4 "$@"
	cp bin/libopenmpt.so "$out/libopenmpt.so.0"
	;;
*)
	# -static folds the compiler's C++ and thread runtimes into the DLL, which Windows does not otherwise have.
	# config-mingw-w64.mk knows no arm64 WINDOWS_ARCH; it only names the cross compilers, which CC, CXX and AR replace.
	case $MSYSTEM in
	CLANGARM64) set -- "$@" CC=clang CXX=clang++ AR=llvm-ar MINGW_COMPILER=clang ;;
	*) set -- "$@" CC=gcc CXX=g++ AR=gcc-ar ;;
	esac
	make -j4 CONFIG=mingw-w64 WINDOWS_ARCH=amd64 LDFLAGS=-static "$@"
	cp bin/libopenmpt.dll "$out/libopenmpt.dll"
	;;
esac
cp LICENSE "$out/LICENSE.libopenmpt"
rm -rf "$tmp"
