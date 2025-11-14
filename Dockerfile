FROM golang:1.22-alpine

# Установим git и ca-certificates
RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Копируем только go.mod (можно игнорировать пустой go.sum)
COPY go.mod ./

# Генерируем go.sum и скачиваем все зависимости
RUN go mod tidy
RUN go mod download

# Копируем весь код проекта
COPY . .

# Сборка бинарника
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/conectwork ./

# Минимальный образ для запуска
FROM alpine:3.18
RUN apk add --no-cache ca-certificates

COPY --from=0 /bin/conectwork /bin/conectwork

USER 1000:1000
ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/bin/conectwork"]
