FROM golang:1.21
WORKDIR /src
RUN apt update; apt install -y protobuf-compiler
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28
ADD . /src
RUN CGO_ENABLED=0 go build

FROM scratch
COPY --from=0 /src/broker-ari /usr/local/bin/broker-ari
ENTRYPOINT ["/usr/local/bin/broker-ari"]
