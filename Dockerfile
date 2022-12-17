FROM golang:1.19 as builder

WORKDIR /src/
COPY *.go go.mod go.sum ./
COPY kafkautils ./kafkautils
COPY message ./message
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o ia2 ./

FROM debian

USER root
RUN apt update && apt install -y nmap openssl tcpdump

COPY --from=builder /src/ia2 /bin/
EXPOSE 8080

CMD ["/bin/ia2"]
