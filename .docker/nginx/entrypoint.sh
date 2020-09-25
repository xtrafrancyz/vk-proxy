#!/bin/ash
envsubst < /tmp/nginx.conf > /etc/nginx/nginx.conf

Check for vk proxy up before starting the nginx
echo "Checking vk proxy status."
until nc -z -v -w30 $VK_PROXY_HOST $VK_PROXY_PORT
do
  echo "Waiting for vk proxy connection..."
  # wait for 5 seconds before check again
  sleep 5
done

echo Startup command: $@
exec "$@"