# Практическая работа №2

## Сборка и публикация

CI повторяет пример на странице 69 PDF (слайд 45): одна задача build_image,
Docker-in-Docker, docker context create, docker buildx create с драйвером
docker-container и одна команда buildx build для linux/arm64 и linux/amd64.
Buildx сам публикует общий multi-arch index; отдельная задача manifest не нужна.

При push коммита задача запускается вручную кнопкой Play, при push Git-тега —
автоматически. Тег образа — Git-тег либо первые 8 символов SHA.
Контекст сборки — точка в конце команды, то есть корень репозитория.

Docker CLI, DinD, BuildKit и базовые образы CI берутся через кафедральный proxy.
TLS отключён по запросу пользователя; настройки подключения задаются в script,
чтобы переменные проекта/группы не вернули порт 2376. Runner должен разрешать DinD.

QEMU и binfmt не устанавливаются. Dockerfile компилирует Go через GOARCH на
BUILDPLATFORM, а финальный этап не выполняет RUN: эмуляция для этих шагов не нужна.
Dockerfile использует Go 1.26.3 → Alpine; Dockerfile.scratch — статический бинарник
и CA-сертификаты. Локально BASE_IMAGE_PREFIX пустой и используется Docker Hub.

## Запуск CI

origin должен указывать на кафедральный GitLab, github — на GitHub:

```bash
git remote -v
# После коммита изменений:
git push origin HEAD
```

Открыть Build → Pipelines и нажать Play у build_image.
Одна задача собирает и публикует обе архитектуры.

Для автоматической сборки выбрать новый Git-тег на нужном коммите:

```bash
git tag v0.2.0
git push origin refs/tags/v0.2.0
```

## Демонстрация Registry и запуска

После ручной сборки коммита и автоматической сборки Git-тега в Container Registry
должны быть два итоговых тега: SHA-8 и Git-тег. Полный путь образа и адрес Registry
скопируйте из GitLab → Deploy → Container Registry.

```bash
REGISTRY='<адрес Registry с портом при наличии>'
IMAGE='<полный путь образа>'
SHA8='<первые 8 символов SHA собранного коммита>'
TAG='v0.2.0'

# Для входа используйте пользователя и токен с read_registry:
docker login "$REGISTRY"
docker buildx imagetools inspect "$IMAGE:$SHA8"
docker buildx imagetools inspect "$IMAGE:$TAG"
# У обоих тегов должны присутствовать linux/amd64 и linux/arm64.

docker pull "$IMAGE:$TAG"
# Задать подключение к доступной из контейнера PostgreSQL:
read -r -s -p 'DATABASE_URL: ' DATABASE_URL
printf '\n'
export DATABASE_URL
docker run --rm --name pepeapi -p 127.0.0.1:8080:8080 \
  -e DATABASE_URL "$IMAGE:$TAG"
```

Перед запуском примените миграции migrations/*.sql через Goose к нужной БД:
сервер сам их не применяет. localhost внутри контейнера означает сам контейнер.
Открыть http://localhost:8080/. Для запуска образа с SHA заменить TAG на SHA8.

## Локальная сборка

С установленным Docker Buildx:

```bash
docker buildx build --load -t pepeapi:local .
docker buildx build --load -f Dockerfile.scratch -t pepeapi:scratch .
```

Для запуска использовать docker run выше с именем локального образа.
