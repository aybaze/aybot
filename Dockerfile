FROM golang AS build

WORKDIR /build

# copy dependency information and fetch them
COPY go.mod ./
RUN go mod download

# copy sources
COPY . .

# build and install (without C-support, otherwise there issue because of the musl glibc replacement on Alpine)
RUN CGO_ENABLED=0 GOOS=linux go build -a .

CMD ["./aybot"]

FROM alpine
# update CA certificates
RUN apk update && apk add ca-certificates

WORKDIR /usr/aybot

COPY --from=build /build/aybot .

CMD ["./aybot"]
