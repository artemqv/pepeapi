# Практическая работа №2

## Сборка и публикация

Dockerfile использует многоэтапную сборку: Go 1.26.3 → Alpine.
Dockerfile.scratch использует тот же статический бинарник и CA-сертификаты,
но не содержит ОС, shell или динамических библиотек. CGO_ENABLED=0 позволяет
запускать приложение без libc; веб-страница встроена через go:embed.

CI состоит из трёх задач:

- build_amd64 — Buildx на Runner с тегом amd64.
- build_arm64 — Buildx на Runner с тегом arm64.
- publish_manifest — объединение образов в общий multi-arch index после обеих сборок.

При push ветки обе сборки запускаются вручную кнопкой Play.
При push Git-тега все задачи выполняются автоматически.
Итоговый тег образа — Git-тег либо первые 8 символов SHA.
Промежуточные теги ci-PIPELINE_ID-ARCH изолируют разные pipeline.
Используйте совместимые с Docker Git-теги, например v0.2.0.

Docker CLI и базовые образы CI загружаются через кафедральный proxy.
Локально BASE_IMAGE_PREFIX пустой, поэтому используется Docker Hub.
QEMU, binfmt и отдельный builder не настраиваются.

## Docker-сервис Runner

По запросу пользователя TLS между Docker CLI и DinD отключён.
В .native_build объявлен один сервис docker:27-dind через кафедральный proxy
с алиасом docker. DOCKER_HOST указывает на tcp://docker:2375;
DOCKER_TLS_CERTDIR, DOCKER_TLS_VERIFY и DOCKER_CERT_PATH заданы пустыми.
Порт daemon не публикуется на хост через эту конфигурацию.

Runner должен разрешать запуск DinD (обычно privileged). Если сервис не стартует,
нужны его логи и проверка настроек Runner у администратора.

## Запуск CI

origin должен указывать на кафедральный GitLab, github — на GitHub:

```bash
git remote -v
# После коммита изменений:
git push origin HEAD
```

Открыть Build → Pipelines и нажать Play у **обеих** сборок.
После успеха обеих автоматически публикуется общий manifest.

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

Scratch-образ локально собран и запущен с PostgreSQL; страница, регистрация и вход
работали. Сборка основного образа остановилась на загрузке Alpine из Docker Hub
из-за TLS timeout. Реальные pipeline и публикация в GitLab ещё не подтверждены.
