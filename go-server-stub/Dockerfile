FROM golang:1.24 AS build
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
COPY go ./go

ENV CGO_ENABLED=0
RUN go build -a -installsuffix cgo -o swagger .

FROM scratch AS runtime
COPY --from=build /app/swagger ./
EXPOSE 8080/tcp
ENTRYPOINT ["./swagger"]
