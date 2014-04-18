#!/bin/bash

# This script will create a local build creating a gopath inside the
# project, downloading all dependencies and putting binaries inside
# bin dir

SCRIPTDIR=$(readlink -f $(dirname $0))

PACKAGE="github.com/sgotti/gomailsync"

if [ ! -h ${SCRIPTDIR}/gopath/src/${PACKAGE} ]; then
   mkdir -p $(dirname ${SCRIPTDIR}/gopath/src/${PACKAGE})
   ln -s ../../../.. ${SCRIPTDIR}/gopath/src/${PACKAGE}
fi

export GOBIN=${SCRIPTDIR}/bin
export GOPATH=${SCRIPTDIR}/gopath

go get ${PACKAGE}
go install ${PACKAGE}

