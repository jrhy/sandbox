#!/bin/sh

set -e

( dockerd > /tmp/dockerd.out 2>&1 ) &
dd if=/dev/zero of=/swapfile bs=1024k count=2048
grep -q 'API listen on' <(tail -f /tmp/dockerd.out)
chmod 600 /swapfile 
mkswap /swapfile 
swapon /swapfile 
docker-compose up

