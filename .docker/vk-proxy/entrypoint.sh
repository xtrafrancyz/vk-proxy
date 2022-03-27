#!/bin/ash

set -e 

/app/vk-proxy -allowMissingConfig \
    -bind 0.0.0.0:$PORT \
    -domain $API_DOMAIN \
    -domain-static $STATIC_DOMAIN \
    -log-verbosity 3
