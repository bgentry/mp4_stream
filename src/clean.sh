#!/bin/sh
set -e

DIRS="
    cmd
    pkg
"

xcd() {
    echo
    cd $1
    echo --- cd $1
}

clean() {
    xcd $1
    gomake clean
}

for dir in $DIRS
do (clean $dir)
done

echo " --- DONE"
