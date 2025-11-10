FROM golang:1.21-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/conectwork ./ 

FROM alpine:3.18
RUN apk add --no-cache ca-certificates
COPY --from=build /bin/conectwork /bin/conectwork
USER 1000:1000
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/bin/conectwork"]
