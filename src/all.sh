#!/bin/sh
set -e

PKGS="
    mp4
"

CMDS="
    mp4_stream
"

xcd() {
    echo
    cd $1
    echo --- cd $1
}

mk() {
    xcd $1
    gomake clean
    gomake
    gomake install
}

rm -rf $GOROOT/pkg/${GOOS}_${GOARCH}/mp4
rm -rf $GOROOT/pkg/${GOOS}_${GOARCH}/mp4.a
rm -rf $GOBIN/mp4_stream

for pkg in $PKGS
do (mk pkg/$pkg)
done

for cmd in $CMDS
do (mk cmd/$cmd)
done

echo " --- DONE"
