FROM nginx:1.19.2-alpine

EXPOSE 8080

ENV PORT 8080
ENV DOMAIN vk-api-proxy.example.com
ENV STATIC_DOMAIN vk-static-proxy.example.com
ENV VK_PROXY_HOST vk-proxy

COPY nginx.conf /etc/nginx/nginx.conf

CMD sed -i 's/80/'"$PORT"'/g' /etc/nginx/nginx.conf && \
    sed -i 's/127.0.0.1:8881/'"$VK_PROXY_HOST"'/g' /etc/nginx/nginx.conf && \
    sed -i 's/vk-api-proxy.example.com/'"$DOMAIN"'/g' /etc/nginx/nginx.conf && \
    sed -i 's/vk-static-proxy.example.com/'"$STATIC_DOMAIN"'/g' /etc/nginx/nginx.conf && \
    nginx -g "daemon off;"