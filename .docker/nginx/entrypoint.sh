#!/bin/ash
set -e 

envsubst "\$PORT \$API_DOMAIN \$STATIC_DOMAIN \$VK_PROXY_HOST \$VK_PROXY_PORT" < /tmp/nginx.conf > /etc/nginx/nginx.conf

echo Check for vk proxy up before starting the nginx
echo "Checking vk proxy status."
until nc -z -v -w30 $VK_PROXY_HOST $VK_PROXY_PORT
do
  echo "Waiting for vk proxy connection..."
  # wait for 5 seconds before check again
  sleep 5
done

echo Startup command: $@
exec "$@"