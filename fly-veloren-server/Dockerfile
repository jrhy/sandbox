FROM alpine AS default
RUN apk add docker docker-compose
COPY docker-compose.yml start.sh /
EXPOSE 14004
ENTRYPOINT /start.sh

