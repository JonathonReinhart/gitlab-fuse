#!/bin/bash

testpath="$1"

echo -en "\ec"
echo "testpath: $testpath"

fusermount -u $testpath >/dev/null 2>&1
go build || exit $?
./gitlab-artifacts-fuse -debug $testpath
