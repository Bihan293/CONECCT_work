FROM golang:1.22-alpine AS build

RUN apk add --no-cache git

WORKDIR /src

# 1. Копируем go.mod И go.sum (даже если go.sum пустой)
COPY go.mod go.sum ./

# 2. Копируем весь код проекта (раньше!)
COPY . .

# 3. Теперь tidy увидит все импорты
RUN go mod tidy
RUN go mod download

# 4. Сборка
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/conectwork ./

# -------------------------
# Minimal runtime image
# -------------------------

FROM alpine:3.18
RUN apk add --no-cache ca-certificates
COPY --from=build /bin/conectwork /bin/conectwork
USER 1000:1000
ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/bin/conectwork"]
