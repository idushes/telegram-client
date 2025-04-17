#!/bin/bash

# Скрипт для сборки и публикации Docker образа
# Автор: dushes

set -e  # Остановка при ошибке

echo "===== Начинаем подготовку к деплою ====="

# Подсказка по авторизации
echo "🔑 Убедитесь, что вы авторизованы в Docker Hub (docker login)"

# Настройка buildx для мультиплатформенной сборки
echo "===== Настройка Docker Buildx ====="
docker buildx use cloud-dushes-builder

echo "===== Начинаем мультиплатформенную сборку и публикацию ====="
docker buildx build --platform linux/amd64,linux/arm64 \
  -t dushes/telegram-client:latest \
  --push .

echo "===== Проверка опубликованных образов ====="
docker buildx imagetools inspect dushes/telegram-client:latest

echo "✅ Деплой успешно завершен!" 