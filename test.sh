#!/bin/bash

if [ $# -lt 1 ]; then
    echo "Usage: $(basename $0) testpath"
    exit 1
fi

testpath="$1"

echo -en "\ec"
echo "testpath: $testpath"

fusermount -u $testpath >/dev/null 2>&1
go build || exit $?
./gitlab-fuse -debug $testpath
