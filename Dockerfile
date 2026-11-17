FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY *.go ./
COPY index.html ./
COPY static ./static
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /arcade-tracker .

FROM alpine:3.20
RUN adduser -D arcade
USER arcade
COPY --from=build /arcade-tracker /usr/local/bin/arcade-tracker
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/arcade-tracker"]
