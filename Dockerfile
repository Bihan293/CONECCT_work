FROM golang:1.22-alpine

# Установить git
RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/conectwork ./

CMD ["/bin/conectwork"]
