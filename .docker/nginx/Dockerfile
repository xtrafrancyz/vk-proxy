FROM nginx:1.19.2-alpine

EXPOSE 8080

ENV PORT 8080

ENV API_DOMAIN vk-api-proxy.example.com
ENV STATIC_DOMAIN vk-static-proxy.example.com
ENV OAUTH_DOMAIN vk-oauth-proxy.example.com

ENV VK_PROXY_HOST vk-proxy
ENV VK_PROXY_PORT 8080


COPY ./entrypoint.sh /entrypoint.sh

COPY nginx.conf /tmp/nginx.conf

ENTRYPOINT [ "/entrypoint.sh" ]
CMD nginx -g "daemon off;"