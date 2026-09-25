FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod ./
COPY . .
RUN go build -o /bin/server ./cmd/server
RUN go build -o /bin/simulator ./cmd/simulator

FROM alpine:3.20
COPY --from=build /bin/server /usr/local/bin/server
COPY --from=build /bin/simulator /usr/local/bin/simulator
EXPOSE 8080
ENTRYPOINT ["server"]
