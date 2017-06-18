#!/bin/sh

rm c.out

set -e

echo mode: set > c.out

function getInternalDeps {

    for p in $(go list -f '{{.Deps}}' $1)
    do
        if [[ $p == github.com/hhrutter/pdflib* ]]; then
            idep=$idep,$p 
        fi
    done
}

echo collecting coverage ...

for q in $(go list ./...)
do
    #echo collecting coverage for $q
    idep=$q
    getInternalDeps $idep
    go test -coverprofile=c1.out -coverpkg=$idep $q && tail -n +2 c1.out  >> c.out
done

rm c1.out

go tool cover -html=c.out