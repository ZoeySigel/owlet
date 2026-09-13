FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/owlet .
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 owlet && mkdir /data && chown owlet /data
COPY --from=build /out/owlet /usr/local/bin/owlet
USER owlet
EXPOSE 8080
ENTRYPOINT ["owlet"]
