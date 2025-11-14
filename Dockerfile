FROM golang:1.22-alpine AS build

RUN apk add --no-cache git

WORKDIR /src

# Копируем только go.mod (go.sum нет в репозитории)
COPY go.mod ./

# Копируем весь код проекта
COPY . .

# Генерируем go.sum и подтягиваем модули
RUN go mod tidy
RUN go mod download

# Сборка
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/conectwork ./

# -------------------------
# Минимальный образ запуска
# -------------------------

FROM alpine:3.18
RUN apk add --no-cache ca-certificates

COPY --from=build /bin/conectwork /bin/conectwork

USER 1000:1000
ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/bin/conectwork"]
