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

# Получаем версию из git тега или используем текущую дату как версию
VERSION=$(date +"%Y.%m.%d-%H.%M")
echo "===== Начинаем мультиплатформенную сборку и публикацию ====="
echo "Версия для публикации: $VERSION"

docker buildx build --platform linux/amd64,linux/arm64 \
  -t dushes/telegram-client:latest \
  -t dushes/telegram-client:$VERSION \
  --push .


echo "✅ Деплой успешно завершен!" 