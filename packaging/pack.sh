#!/bin/bash

if [ ! -f control.template ]; then
    echo "File 'control.template' not found. Please create an appropriate template"
fi

read -p 'Major Version: ' MAJOR_VERSION
read -p 'Minor Version: ' MINOR_VERSION
read -p 'Patch Number: ' PATCH
read -p 'Archotecture(like amd64, i386): ' ARCH
VERSION=${MAJOR_VERSION}.${MINOR_VERSION}-${PATCH}
dir=dunner_$VERSION

echo Creating dir "${dir}" and required sub-directories in it
mkdir $dir
mkdir $dir/usr
mkdir $dir/usr/local
mkdir $dir/usr/local/bin

echo "Building binary from  source code"
go build -o $dir/usr/local/bin/dunner ..

echo "Populating data in template"
mkdir $dir/DEBIAN
cp control.template $dir/DEBIAN/control
sed -i s/#version#/${VERSION}/g $dir/DEBIAN/control
sed -i s/#arch#/${ARCH}/g $dir/DEBIAN/control

echo "Building a .deb file"
dpkg-deb --build $dir
