FROM composer:2 AS vendor

WORKDIR /app
COPY composer.json composer.lock symfony.lock ./
RUN composer install --no-dev --no-scripts --prefer-dist --no-progress --optimize-autoloader

FROM dunglas/frankenphp:1-php8.4-alpine

ENV APP_ENV=prod APP_DEBUG=0 SERVER_NAME=:8000

WORKDIR /app
COPY . .
COPY --from=vendor /app/vendor vendor
RUN php bin/console cache:warmup

EXPOSE 8000
