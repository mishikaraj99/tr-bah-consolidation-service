FROM golang:1.25-alpine AS build_base
RUN apk add --no-cache git
WORKDIR /tmp/traya-bah-service
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o ./out/traya-bah-service .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build_base /tmp/traya-bah-service/out/traya-bah-service /app/traya-bah-service
EXPOSE 3000
CMD ["/app/traya-bah-service"]
